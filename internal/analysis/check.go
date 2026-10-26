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

// errDataTypeUnknown is reported for an ExecuteTemplate call whose data
// argument has no type because the package does not type check.
var errDataTypeUnknown = errors.New("the data argument has no type because the package does not type check; run go build (a stale generated file can cause this)")

type CheckConfiguration struct {
	Verbose bool

	// AllowUnused lists templates nothing executes as a note instead of
	// failing: a project may render some templates outside muxt, through
	// a helper whose template name is not a literal. Route templates with
	// no generated handler still fail.
	AllowUnused bool

	TemplatesVariables []string
}

// CheckResult counts the ExecuteTemplate calls Check found, so the caller
// can report them on success.
type CheckResult struct {
	// Checked is how many calls were type checked.
	Checked int

	// NonLiteral is how many were not, because their template name is not
	// a string literal.
	NonLiteral int
}

// Check validates the package's templates. It logs each ExecuteTemplate
// call it cannot check because the template name is not a string literal:
// the templates such a call renders are otherwise left unchecked without
// a word.
func Check(config CheckConfiguration, logger *log.Logger, pkg source.Package) (CheckResult, error) {
	var errs []error
	var result CheckResult
	qualifier := astgen.NewTypeFormatter(pkg.Types.Path()).Qualifier

	for _, lt := range pkg.Variables {
		global := newGlobal(pkg, lt)

		if err := reportDefinitionErrors(logger, lt); err != nil {
			errs = append(errs, err)
		}

		for _, position := range lt.NonLiteralCalls {
			result.NonLiteral++
			logger.Printf("%s: %s not checked: the template name is not a string literal, so which template it renders is not known until run time", position, executeTemplateFunc)
		}

		executedTemplates := make(map[string][]TemplateExecution)
		var diagnostics typeErrors
		// walkedEveryCall is false once a call cannot be walked, and then
		// what the calls reach is unknown, so nothing is reported unused.
		// A call whose template name is not a literal may render any
		// template, so it is one of those.
		walkedEveryCall := len(lt.NonLiteralCalls) == 0
		for _, c := range lt.Calls {
			result.Checked++
			if config.Verbose {
				logger.Println("checking endpoint", c.Template)
			}
			if c.Data == nil {
				// The type checker recorded no type for the data argument,
				// as when the call is in code it skipped over a
				// redeclaration, so there is no type of dot to check with.
				err := errDataTypeUnknown
				logger.Println(c.Position, executeTemplateFunc, strconv.Quote(c.Template))
				logger.Println(" - ", err)
				errs = append(errs, err)
				walkedEveryCall = false
				continue
			}
			err := findTemplateExecution(executedTemplates, global, qualifier, lt.Set, c.Position, c.Template, c.Data)
			if err != nil {
				errs = append(errs, diagnostics.report(logger, config.Verbose, qualifier, c, err)...)
			}
		}
		diagnostics.reportOtherCalls(logger)

		if walkedEveryCall {
			errs = append(errs, reportUnusedTemplates(logger, lt.Set, executedTemplates, config.AllowUnused)...)
		}
	}

	switch len(errs) {
	case 0:
		return result, nil
	case 1:
		return result, fmt.Errorf("1 error")
	default:
		return result, fmt.Errorf("%d errors", len(errs))
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

// typeErrors logs the errors the ExecuteTemplate calls of one templates
// variable reach. A partial several calls render with the same type of dot
// fails the same way from each, so an error is logged once, under the
// first call that reaches it, rather than once per call; the other calls
// are listed after the last call is checked.
type typeErrors struct {
	byText map[string]*typeError
	// order lists the errors in the order they were first logged.
	order []*typeError
}

type typeError struct {
	// line is the one-line error; detail is what DetailedError adds after
	// it: the signature of a function called wrongly, or the declaration
	// of the type a field or method was looked up on.
	line, detail string
	// declaration is set when detail is a type declaration. A struct's
	// field list can run long and buries the error, so it is logged only
	// with -v.
	declaration bool
	err         error
	// calls are the calls that reached the error, formatted by callLine,
	// each once.
	calls []string
}

// report logs the errors in err that no earlier call reached, under call,
// and returns one error per error logged. Errors an earlier call reached
// are remembered for reportOtherCalls. A type declaration is logged only
// when verbose.
func (te *typeErrors) report(logger *log.Logger, verbose bool, qualifier types.Qualifier, call source.Call, err error) []error {
	if te.byText == nil {
		te.byText = make(map[string]*typeError)
	}
	callText := callLine(qualifier, call)
	var errs []error
	endsWithBlank := true
	for _, e := range splitTypeError(qualifier, err) {
		key := e.line + "\n" + e.detail
		if seen, ok := te.byText[key]; ok {
			// A call that renders a partial twice reaches its error twice.
			if !slices.Contains(seen.calls, callText) {
				seen.calls = append(seen.calls, callText)
			}
			continue
		}
		e.calls = []string{callText}
		te.byText[key] = e
		te.order = append(te.order, e)
		errs = append(errs, e.err)
		if len(errs) == 1 {
			logger.Println(callText)
		}
		logger.Println(e.line)
		endsWithBlank = false
		if e.detail != "" && (verbose || !e.declaration) {
			logger.Println()
			logger.Println(e.detail)
			logger.Println()
			endsWithBlank = true
		}
	}
	if !endsWithBlank {
		logger.Println()
	}
	return errs
}

// reportOtherCalls logs, for each error more than one call reached, the
// calls after the first.
func (te *typeErrors) reportOtherCalls(logger *log.Logger) {
	for _, e := range te.order {
		if len(e.calls) < 2 {
			continue
		}
		logger.Println(e.line)
		logger.Println("  also reached from:")
		for _, c := range e.calls[1:] {
			logger.Println("    " + c)
		}
		logger.Println()
	}
}

// splitTypeError returns one typeError per independent failure in err.
func splitTypeError(qualifier types.Qualifier, err error) []*typeError {
	checkErr, ok := errors.AsType[*check.Error](err)
	if !ok {
		return []*typeError{{line: " -  " + err.Error(), err: err}}
	}
	var result []*typeError
	for leaf := range checkErr.All {
		if leaf.Type == check.ErrorTypeAggregate {
			continue
		}
		var sb strings.Builder
		if detailErr := leaf.DetailedError(&sb, qualifier); detailErr != nil {
			// The compact error must still reach the user.
			result = append(result, &typeError{line: leaf.Error(), err: leaf})
			continue
		}
		line, detail, _ := strings.Cut(sb.String(), "\n\n")
		_, declaration := errors.AsType[*check.IdentifierError](leaf)
		result = append(result, &typeError{line: line, detail: detail, declaration: declaration, err: leaf})
	}
	return result
}

func callLine(qualifier types.Qualifier, call source.Call) string {
	return fmt.Sprintf("%s %s %s %s", call.Position, executeTemplateFunc, strconv.Quote(call.Template), types.TypeString(call.Data, qualifier))
}

// reportUnusedTemplates logs the templates no ExecuteTemplate call reaches
// and returns one error per kind. With allowUnused, unused templates that
// are not routes are listed without an error.
func reportUnusedTemplates(logger *log.Logger, ts *template.Template, executedTemplates map[string][]TemplateExecution, allowUnused bool) []error {
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
	switch {
	case len(unusedPartials) == 0:
	case allowUnused:
		logger.Println("Unused templates (allowed by --allow-unused):")
		logTemplatePositions(logger, ts, unusedPartials)
	default:
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
// are not reported: a listing shows what the walk reached. A call with no
// data type, in code the type checker skipped, is not walked.
func executeTemplateTree(global *check.Global, ts *template.Template, name string, data types.Type) {
	if data == nil {
		return
	}
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
