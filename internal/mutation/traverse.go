package mutation

import (
	"go/token"
	"go/types"
	"text/template/parse"

	"github.com/typelate/check"

	"github.com/typelate/muxt/internal/asteval"
)

// callSite is one templates.ExecuteTemplate call, which is where a
// template's dot gets a concrete type.
//
// A template action means nothing on its own: {{.Total}} is only a field
// access because of what dot is where the template is rendered. Mutating
// in the context of a call is what lets the engine know that.
type callSite struct {
	Position token.Position
	Template string
	DataType types.Type
}

// scope is one template reached from a call site, with the type of dot
// flowing into it.
type scope struct {
	call     callSite
	template string
	dataType types.Type

	// via is set when the template was reached through a {{template}}
	// invocation rather than named by the call itself.
	via bool

	src  *templateSource
	tree *parse.Tree

	// identity is the template's own source.
	identity string
}

// treeLocation pairs a parsed template with the source its text came
// from, so a mutation found in the tree can be written back to a file.
type treeLocation struct {
	src  *templateSource
	tree *parse.Tree

	// identity is the template's own source, which is what decides
	// whether its mutants have to be run again.
	identity string
}

// trim is a subtree the traversal did not descend into, because the same
// template had already been reached with the same type of dot.
//
// It is reported so that a shorter run is legible rather than mysterious:
// the work was skipped because it would have repeated, not because the
// template was overlooked.
type trim struct {
	call     callSite
	template string
	dataType types.Type
	via      bool
	firstFor callSite
}

// traverse walks the templates reachable from each ExecuteTemplate call,
// depth first, carrying the type of dot into each one.
//
// Depth first is what keeps a report readable: everything about one
// template, and then the partials it renders, before moving to the next
// call.
//
// A template is mutated once per type of dot it is rendered with, across
// the whole run. Two calls rendering the same template with the same
// input, or two {{template}} invocations passing the same type, would
// produce the same mutants and the same verdicts, so the second is
// trimmed.
func traverse(lt *asteval.LoadedTemplates, index map[string]treeLocation) ([]scope, []trim) {
	var (
		scopes  []scope
		trimmed []trim
	)
	visited := make(map[string]callSite)
	for call := range lt.Templates.ExecuteTemplateCalls() {
		site := callSite{
			Position: lt.Package.Fset.Position(call.Call.Pos()),
			Template: call.TemplateName,
			DataType: call.DataType,
		}
		visit(lt, index, site, call.TemplateName, call.DataType, false, visited, &scopes, &trimmed)
	}
	return scopes, trimmed
}

func visit(lt *asteval.LoadedTemplates, index map[string]treeLocation, site callSite, name string, dot types.Type, via bool, visited map[string]callSite, out *[]scope, trimmed *[]trim) {
	key := executionKey(name, dot)
	if first, seen := visited[key]; seen {
		*trimmed = append(*trimmed, trim{
			call:     site,
			template: name,
			dataType: dot,
			via:      via,
			firstFor: first,
		})
		return
	}
	visited[key] = site

	location, ok := index[name]
	if !ok {
		return
	}
	*out = append(*out, scope{
		call:     site,
		template: name,
		dataType: dot,
		via:      via,
		src:      location.src,
		tree:     location.tree,
		identity: location.identity,
	})

	for _, nested := range templateCalls(lt, location.tree, dot) {
		visit(lt, index, site, nested.name, nested.dot, true, visited, out, trimmed)
	}
}

type templateCall struct {
	name string
	dot  types.Type
}

// templateCalls reports the {{template}} invocations in tree, with the
// type of dot each one passes on.
//
// The types come from the checker, which is the only thing that knows how
// dot narrows through a range or a with on the way to the invocation.
func templateCalls(lt *asteval.LoadedTemplates, tree *parse.Tree, dot types.Type) []templateCall {
	var found []templateCall
	lt.Global.InspectTemplateNode = func(node *parse.TemplateNode, _ *parse.Tree, tp types.Type, _ check.Definition) {
		found = append(found, templateCall{name: node.Name, dot: tp})
	}
	// A template that does not check still yields the invocations found
	// before the failure, which is better than none: muxt check is where
	// a type error should be reported, not here.
	_ = check.Execute(lt.Global, tree, dot)
	lt.Global.InspectTemplateNode = nil
	return found
}

// checks reports whether tree type checks with dot, which is how a mutant
// is told from a mutation that merely breaks the template.
func checks(lt *asteval.LoadedTemplates, tree *parse.Tree, dot types.Type) bool {
	inspector := lt.Global.InspectTemplateNode
	lt.Global.InspectTemplateNode = nil
	err := check.Execute(lt.Global, tree, dot)
	lt.Global.InspectTemplateNode = inspector
	return err == nil
}

// typeKey identifies a type exactly, for deciding whether a template has
// already been mutated with this dot. It keeps full package paths so that
// two same named types from different packages never collide.
func typeKey(t types.Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.String()
}

// typeDisplay renders a type for a report, qualified by package name
// rather than by import path.
//
// A generic route type written out in full is most of a line of import
// path, which buries the part a reader is actually looking at.
func typeDisplay(t types.Type) string {
	if t == nil {
		return "<nil>"
	}
	return types.TypeString(t, func(p *types.Package) string { return p.Name() })
}

// complexity is the cyclomatic complexity of a template: one, plus one
// for every point the rendering can take a different path.
//
// It is reported because it predicts the work ahead. Every branch is
// somewhere a mutation applies, so a template set's total complexity and
// the number of mutants to run move together.
func complexity(node parse.Node) int {
	return 1 + branchPoints(node)
}

func branchPoints(node parse.Node) int {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return 0
		}
		total := 0
		for _, child := range n.Nodes {
			total += branchPoints(child)
		}
		return total
	case *parse.IfNode:
		return 1 + pipelineBranches(n.Pipe) + branchPoints(n.List) + branchPoints(n.ElseList)
	case *parse.RangeNode:
		return 1 + pipelineBranches(n.Pipe) + branchPoints(n.List) + branchPoints(n.ElseList)
	case *parse.WithNode:
		return 1 + pipelineBranches(n.Pipe) + branchPoints(n.List) + branchPoints(n.ElseList)
	default:
		return 0
	}
}

// pipelineBranches counts the short circuiting operators in a pipeline,
// each of which is a path the rendering can take.
func pipelineBranches(pipe *parse.PipeNode) int {
	if pipe == nil {
		return 0
	}
	total := 0
	for _, cmd := range pipe.Cmds {
		for _, arg := range cmd.Args {
			ident, ok := arg.(*parse.IdentifierNode)
			if !ok {
				continue
			}
			if ident.Ident == "and" || ident.Ident == "or" {
				total++
			}
		}
	}
	return total
}
