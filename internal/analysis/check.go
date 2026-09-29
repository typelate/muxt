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
func Check(config CheckConfiguration, logger *log.Logger, pkg source.Package) (int, error) {
	var errs []error
	totalChecked := 0
	qualifier := astgen.NewTypeFormatter(pkg.Types.Path()).Qualifier

	for _, lt := range pkg.Variables {
		global := newGlobal(pkg, lt)

		if err := reportDefinitionErrors(logger, lt); err != nil {
			errs = append(errs, err)
		}

		executedTemplates := make(map[string][]TemplateExecution)
		for _, c := range lt.Calls {
			totalChecked++
			if config.Verbose {
				logger.Println("checking endpoint", c.Template)
			}
			err := findTemplateExecution(executedTemplates, global, qualifier, lt.Set, c.Position, c.Template, c.Data)
			if err != nil {
				reportExecutionError(logger, qualifier, c, err)
				errs = append(errs, err)
			}
		}

		errs = append(errs, reportUnusedTemplates(logger, lt.Set, executedTemplates)...)
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

// reportDefinitionErrors validates route template names so a malformed name
// surfaces with its position instead of leaving the template to be reported
// as merely unused.
func reportDefinitionErrors(logger *log.Logger, variable source.Variable) error {
	_, err := muxt.Definitions(variable)
	if err == nil {
		return nil
	}
	if multiLine, ok := errors.AsType[muxt.MultiLineError](err); ok {
		logger.Println(multiLine.MultiLineError())
		logger.Println()
	} else {
		logger.Println(err)
	}
	return err
}

func reportExecutionError(logger *log.Logger, qualifier types.Qualifier, call source.Call, err error) {
	logger.Println(call.Position, executeTemplateFunc, strconv.Quote(call.Template), types.TypeString(call.Data, qualifier))
	if checkErr, ok := errors.AsType[*check.Error](err); ok {
		var sb strings.Builder
		if detailErr := checkErr.DetailedError(&sb, qualifier); detailErr != nil {
			// The compact error must still reach the user.
			logger.Println(" - ", err)
		}
		logger.Println(sb.String())
	} else {
		logger.Println(" - ", err)
	}
	logger.Println()
}

// reportUnusedTemplates logs the templates no ExecuteTemplate call reaches
// and returns one error per kind.
func reportUnusedTemplates(logger *log.Logger, ts *template.Template, executedTemplates map[string][]TemplateExecution) []error {
	var errs []error
	unusedRoutes, unusedPartials := partitionUnusedTemplates(ts, findUnusedTemplates(ts, executedTemplates))
	if len(unusedRoutes) > 0 {
		// Route templates with no ExecuteTemplate caller usually mean
		// the generated routes file is missing or stale, not that the
		// template should be deleted.
		logger.Println("Route templates with no generated handler; run muxt generate to wire them up:")
		logTemplatePositions(logger, ts, unusedRoutes)
		errs = append(errs, fmt.Errorf("%d route templates are not wired to generated handlers", len(unusedRoutes)))
	}
	if len(unusedPartials) > 0 {
		logger.Println("Unused templates:")
		logTemplatePositions(logger, ts, unusedPartials)
		errs = append(errs, fmt.Errorf("unused templates %d", len(unusedPartials)))
	}
	return errs
}

func logTemplatePositions(logger *log.Logger, ts *template.Template, names []string) {
	for _, name := range names {
		t := ts.Lookup(name)
		logger.Printf("  - %s: %q", check.ParseNodePosition(t.Tree, t.Tree.Root), name)
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
		followTemplate(ts, n.Name, seen)
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

func followTemplate(ts *template.Template, name string, seen map[string]bool) {
	if seen[name] {
		return
	}
	seen[name] = true
	if t := ts.Lookup(name); t != nil && t.Tree != nil {
		collectTemplateReferences(ts, t.Tree.Root, seen)
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

	allNames := make(map[string]bool)
	for _, t := range allTemplates {
		allNames[t.Name()] = true
	}

	usedTemplates := make(map[string]bool)
	for name := range executedTemplates {
		usedTemplates[name] = true
	}

	// A file template holding only define blocks is empty once they are stripped.
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

// executeTemplateTree walks the template called name with data so that
// global's InspectTemplateNode sees each {{template}} action. Type errors
// are not reported: a listing shows what the walk reached.
func executeTemplateTree(global *check.Global, ts *template.Template, name string, data types.Type) {
	if t := ts.Lookup(name); t != nil && t.Tree != nil {
		_ = check.Execute(global, t.Tree, data)
	}
}

// newGlobal wires a check.Global for type checking a templates variable's
// templates in pkg.
//
// Global.Definitions stays nil on purpose: it feeds the check.Definition an
// InspectTemplateNode callback is handed, and every callback here ignores
// it. Where a definition was written comes from source.Variable, which
// muxt.Definitions and the mutation planner read.
func newGlobal(pkg source.Package, variable source.Variable) *check.Global {
	return check.NewGlobal(pkg.Types, pkg.Fset, check.FindTreeFunc(func(name string) (*parse.Tree, bool) {
		t := variable.Set.Lookup(name)
		if t == nil || t.Tree == nil {
			return nil, false
		}
		return t.Tree, true
	}), check.Functions(variable.Functions))
}
