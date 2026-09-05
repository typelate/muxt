package analysis

import (
	"errors"
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

// executeTemplateFunc names the method the endpoint scan reports call
// positions for.
const executeTemplateFunc = "ExecuteTemplate"

type CheckConfiguration struct {
	Verbose            bool
	TemplatesVariables []string
}

func Check(config CheckConfiguration, wd string, log *log.Logger, fileSet *token.FileSet, pl []*packages.Package) error {
	routesPkg, ok := asteval.PackageAtFilepath(pl, wd)
	if !ok {
		return asteval.NoPackageError(wd, pl)
	}

	var errs []error

	for _, tv := range config.TemplatesVariables {
		lt, err := asteval.LoadTemplates(wd, tv, pl)
		if err != nil {
			return err
		}
		global, ts := lt.Global, lt.HTML

		executedTemplates := make(map[string][]TemplateExecution)

		for c := range lt.Templates.ExecuteTemplateCalls() {
			templateName, dataType := c.TemplateName, c.DataType
			if config.Verbose {
				log.Println("checking endpoint", templateName)
			}
			qualifier := astgen.NewTypeFormatter(routesPkg.PkgPath).Qualifier
			if err := findTemplateExecution(executedTemplates, global, fileSet, qualifier, ts, c.Call, templateName, dataType); err != nil {
				log.Println(fileSet.Position(c.Call.Pos()), executeTemplateFunc, strconv.Quote(templateName), types.TypeString(dataType, qualifier))
				if checkErr, ok := errors.AsType[*check.Error](err); ok {
					var sb strings.Builder
					if detailErr := checkErr.DetailedError(&sb, qualifier); detailErr != nil {
						// The detail rendering failed; the compact error
						// must still reach the user.
						log.Println(" - ", err)
					}
					log.Println(sb.String())
				} else {
					log.Println(" - ", err)
				}
				log.Println()
				errs = append(errs, err)
			}
		}

		unusedTemplates := findUnusedTemplates(ts, executedTemplates)
		if len(unusedTemplates) > 0 {
			log.Println("Unused templates:")
			for _, name := range unusedTemplates {
				t := ts.Lookup(name)
				log.Printf("  - %s: %q", check.ParseNodePosition(t.Tree, t.Tree.Root), name)
			}
			errs = append(errs, fmt.Errorf("unused templates %d", len(unusedTemplates)))
		}
	}

	switch len(errs) {
	case 0:
		if config.Verbose {
			log.Println(`OK`)
		}
		return nil
	case 1:
		return fmt.Errorf("1 error")
	default:
		return fmt.Errorf("%d errors", len(errs))
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
	global.InspectTemplateNode = func(node *parse.TemplateNode, tree *parse.Tree, tp types.Type, _ check.Definition) {
		executedTemplates[node.Name] = append(executedTemplates[node.Name], newTemplateExecution(check.ParseNodePosition(tree, node), node, node.Name, dataType))
	}
	global.Qualifier = qualifier
	if err := check.Execute(global, tree, dataType); err != nil {
		return err
	}
	return nil
}
