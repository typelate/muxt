package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"html/template"
	"log"
	"slices"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
	"github.com/typelate/muxt/internal/astgen"
)

type CheckConfiguration struct {
	Verbose           bool
	TemplatesVariable string
}

func Check(config CheckConfiguration, wd string, log *log.Logger, fileSet *token.FileSet, pl []*packages.Package) error {
	routesPkg, ok := asteval.PackageAtFilepath(pl, wd)
	if !ok {
		return fmt.Errorf("package not found at %s", wd)
	}

	ts, fm, err := asteval.Templates(wd, config.TemplatesVariable, routesPkg)
	if err != nil {
		return err
	}
	fns := check.DefaultFunctions(routesPkg.Types)
	fns = fns.Add(check.Functions(fm))

	global := check.NewGlobal(routesPkg.Types, routesPkg.Fset, asteval.NewForrest(ts), fns)

	// Track which templates are executed via ExecuteTemplate calls
	executedTemplates := make(map[string][]TemplateExecution)

	var errs []error
	for _, file := range routesPkg.Syntax {
		for node := range ast.Preorder(file) {
			templateName, dataType, ok := asteval.ExecuteTemplateArguments(node, routesPkg.TypesInfo, config.TemplatesVariable)
			if !ok {
				continue
			}
			if config.Verbose {
				log.Println("checking endpoint", templateName)
			}
			qualifier := astgen.NewTypeFormatter(routesPkg.PkgPath).Qualifier
			if err := findTemplateExecution(executedTemplates, global, fileSet, qualifier, ts, node, templateName, dataType); err != nil {
				log.Println(fileSet.Position(node.Pos()), asteval.TemplateExecuteFunc, strconv.Quote(templateName), types.TypeString(dataType, qualifier))
				log.Println(" - ", err)
				log.Println()
				errs = append(errs, err)
			}
		}
	}
	unusedTemplates := findUnusedTemplates(ts, executedTemplates)
	if len(unusedTemplates) > 0 {
		log.Println("Unused templates:")
		for _, name := range unusedTemplates {
			t := ts.Lookup(name)
			log.Printf("  - %s: %q", asteval.NewParseNodePosition(t.Tree, t.Tree.Root), name)
		}
		errs = append(errs, fmt.Errorf("unused templates %d", len(unusedTemplates)))
	}
	switch len(errs) {
	case 1:
		return fmt.Errorf("1 error")
	default:
		return fmt.Errorf("%d errors", len(errs))
	case 0:
		if config.Verbose {
			log.Println(`OK`)
		}
		return nil
	}
}

// findUnusedTemplates returns a list of template names that are defined but never used.
// A template is considered "used" if it:
// 1. Is executed via ExecuteTemplate calls in the code
// 2. Is referenced via {{template "name"}} from a used template
func findUnusedTemplates(ts *template.Template, executedTemplates map[string][]TemplateExecution) []string {
	allTemplates := ts.Templates()
	if len(allTemplates) == 0 {
		return nil
	}

	// Collect all template names
	allNames := make(map[string]bool)
	for _, t := range allTemplates {
		allNames[t.Name()] = true
	}

	// Build a set of used templates starting from executed templates
	usedTemplates := make(map[string]bool)
	for name := range executedTemplates {
		usedTemplates[name] = true
	}

	// Find unused templates (skip templates that are empty after define blocks are stripped)
	var unused []string
	for name := range allNames {
		if !usedTemplates[name] {
			t := ts.Lookup(name)
			if t != nil && t.Tree != nil && !isEmptyTemplate(t.Tree.Root) {
				unused = append(unused, name)
			}
		}
	}

	slices.Sort(unused)
	return unused
}

// isEmptyTemplate returns true if the template tree contains only whitespace and comments
// (e.g., a file template that only contains define blocks)
func isEmptyTemplate(node parse.Node) bool {
	if node == nil {
		return true
	}

	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return true
		}
		for _, child := range n.Nodes {
			if !isEmptyTemplate(child) {
				return false
			}
		}
		return true

	case *parse.TextNode:
		return strings.TrimSpace(string(n.Text)) == ""

	case *parse.CommentNode:
		return true

	default:
		// Any other node type (actions, if, range, etc.) is non-empty
		return false
	}
}

type TemplateExecution struct {
	token.Position
	nd   any
	tp   types.Type
	Name string
	Type string
}

func newTemplateExecution(pos token.Position, n any, templateName string, dataType types.Type) TemplateExecution {
	return TemplateExecution{
		tp:       dataType,
		nd:       n,
		Name:     templateName,
		Type:     dataType.String(),
		Position: pos,
	}
}

func findTemplateExecution(executedTemplates map[string][]TemplateExecution, global *check.Global, fileSet *token.FileSet, qualifier types.Qualifier, ts *template.Template, node ast.Node, templateName string, dataType types.Type) error {
	executedTemplates[templateName] = append(executedTemplates[templateName], newTemplateExecution(fileSet.Position(node.Pos()), node, templateName, dataType))
	ts2 := ts.Lookup(templateName)
	if ts2 == nil {
		return fmt.Errorf("template %q not found", templateName)
	}
	tree := ts2.Tree
	global.InspectTemplateNode = func(tree *parse.Tree, node *parse.TemplateNode, tp types.Type) {
		executedTemplates[node.Name] = append(executedTemplates[node.Name], newTemplateExecution(asteval.NewParseNodePosition(tree, node), node, node.Name, dataType))
	}
	global.Qualifier = qualifier
	if err := check.Execute(global, tree, dataType); err != nil {
		return err
	}
	return nil
}
