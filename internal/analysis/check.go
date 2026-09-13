package analysis

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"html/template"
	"log"
	"slices"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/typelate/check"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

// executeTemplateFunc names the method the endpoint scan reports call
// positions for.
const executeTemplateFunc = "ExecuteTemplate"

type CheckConfiguration struct {
	Verbose            bool
	TemplatesVariables []string
}

// Check validates the package's templates and returns how many
// ExecuteTemplate call sites it checked, so the caller can report the
// count on success.
func Check(config CheckConfiguration, log *log.Logger, pkg source.Package) (int, error) {
	var errs []error
	totalChecked := 0

	for _, lt := range pkg.Variables {
		global, ts := newGlobal(pkg, lt), lt.Set

		// Route template names are validated here so a malformed name
		// surfaces with its position instead of leaving the template to
		// be reported as merely unused below.
		if _, err := muxt.Definitions(lt); err != nil {
			if multiLine, ok := errors.AsType[muxt.MultiLineError](err); ok {
				log.Println(multiLine.MultiLineError())
				log.Println()
			} else {
				log.Println(err)
			}
			errs = append(errs, err)
		}

		executedTemplates := make(map[string][]TemplateExecution)
		checkedTemplates := 0

		for _, c := range lt.Calls {
			checkedTemplates++
			templateName, dataType := c.Template, c.Data
			if config.Verbose {
				log.Println("checking endpoint", templateName)
			}
			qualifier := astgen.NewTypeFormatter(pkg.Types.Path()).Qualifier
			if err := findTemplateExecution(executedTemplates, global, qualifier, ts, c.Position, templateName, dataType); err != nil {
				log.Println(c.Position, executeTemplateFunc, strconv.Quote(templateName), types.TypeString(dataType, qualifier))
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
		unusedRoutes, unusedPartials := partitionUnusedTemplates(ts, unusedTemplates)
		if len(unusedRoutes) > 0 {
			// Route templates with no ExecuteTemplate caller usually mean
			// the generated routes file is missing or stale, not that the
			// template should be deleted.
			log.Println("Route templates with no generated handler; run muxt generate to wire them up:")
			for _, name := range unusedRoutes {
				t := ts.Lookup(name)
				log.Printf("  - %s: %q", check.ParseNodePosition(t.Tree, t.Tree.Root), name)
			}
			errs = append(errs, fmt.Errorf("%d route templates are not wired to generated handlers", len(unusedRoutes)))
		}
		if len(unusedPartials) > 0 {
			log.Println("Unused templates:")
			for _, name := range unusedPartials {
				t := ts.Lookup(name)
				log.Printf("  - %s: %q", check.ParseNodePosition(t.Tree, t.Tree.Root), name)
			}
			errs = append(errs, fmt.Errorf("unused templates %d", len(unusedPartials)))
		}
		totalChecked += checkedTemplates
	}

	switch len(errs) {
	case 0:
		return totalChecked, nil
	case 1:
		return totalChecked, fmt.Errorf("1 error")
	default:
		return totalChecked, fmt.Errorf("%d errors", len(errs))
	}
}

// partitionUnusedTemplates splits the unused template names into route
// templates (whose fix is running muxt generate) and plain templates.
// A plain template referenced from a route template's tree is pending
// that route's wiring rather than unused, so it is omitted entirely:
// its route is already reported.
func partitionUnusedTemplates(ts *template.Template, unused []string) (routes, partials []string) {
	pending := make(map[string]bool)
	for _, t := range ts.Templates() {
		if muxt.IsRouteDefinitionName(t.Name()) && t.Tree != nil {
			collectTemplateReferences(ts, t.Tree.Root, pending)
		}
	}
	for _, name := range unused {
		switch {
		case muxt.IsRouteDefinitionName(name):
			routes = append(routes, name)
		case pending[name]:
		default:
			partials = append(partials, name)
		}
	}
	return routes, partials
}

// collectTemplateReferences records every template name reachable from
// node through {{template}} actions, following references transitively.
func collectTemplateReferences(ts *template.Template, node parse.Node, seen map[string]bool) {
	switch n := node.(type) {
	case nil:
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			collectTemplateReferences(ts, child, seen)
		}
	case *parse.TemplateNode:
		if seen[n.Name] {
			return
		}
		seen[n.Name] = true
		if t := ts.Lookup(n.Name); t != nil && t.Tree != nil {
			collectTemplateReferences(ts, t.Tree.Root, seen)
		}
	case *parse.IfNode:
		collectTemplateReferences(ts, n.List, seen)
		collectTemplateReferences(ts, n.ElseList, seen)
	case *parse.RangeNode:
		collectTemplateReferences(ts, n.List, seen)
		collectTemplateReferences(ts, n.ElseList, seen)
	case *parse.WithNode:
		collectTemplateReferences(ts, n.List, seen)
		collectTemplateReferences(ts, n.ElseList, seen)
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

func findTemplateExecution(executedTemplates map[string][]TemplateExecution, global *check.Global, qualifier types.Qualifier, ts *template.Template, position token.Position, templateName string, dataType types.Type) error {
	executedTemplates[templateName] = append(executedTemplates[templateName], newTemplateExecution(position, nil, templateName, dataType))
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

// newGlobal wires a check.Global for type checking a templates variable's
// templates in pkg.
func newGlobal(pkg source.Package, variable source.Variable) *check.Global {
	return check.NewGlobal(pkg.Types, pkg.Fset, check.FindTreeFunc(func(name string) (*parse.Tree, bool) {
		t := variable.Set.Lookup(name)
		if t == nil || t.Tree == nil {
			return nil, false
		}
		return t.Tree, true
	}), check.Functions(variable.Functions))
}
