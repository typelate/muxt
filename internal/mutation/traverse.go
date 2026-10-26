package mutation

import (
	"go/token"
	"go/types"
	"text/template/parse"

	"github.com/typelate/check"
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

	// treeLocation is where the template was found. It is embedded
	// rather than copied field by field so that a scope cannot come to
	// hold a tree from one source and positions from another.
	treeLocation
}

// treeLocation pairs a parsed template with the source its text came
// from, so a mutation found in the tree can be written back to a file.
type treeLocation struct {
	src  *templateSource
	tree *parse.Tree
}

// executionKey names a template rendered with one type of dot, which is
// the unit mutated once per run.
//
// Neither a template name nor a type's string can hold a NUL, so no two
// pairs run together into one key.
func executionKey(name string, dot types.Type) string {
	return name + "\x00" + typeKey(dot)
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
func traverse(lt *checked, index map[string]treeLocation) ([]scope, []trim) {
	t := &traversal{lt: lt, index: index, visited: make(map[string]callSite)}
	for _, call := range lt.Calls {
		site := callSite{
			Position: call.Position,
			Template: call.Template,
			DataType: call.Data,
		}
		t.visit(site, call.Template, call.Data, false)
	}
	return t.scopes, t.trimmed
}

// traversal is the state of one walk: what it reads from, and what it has
// found so far.
type traversal struct {
	lt    *checked
	index map[string]treeLocation

	// visited records the call site each template and dot was first
	// reached from, which a trim reports.
	visited map[string]callSite
	scopes  []scope
	trimmed []trim
}

// visit records the template name reached from site with dot, then the
// templates it invokes.
func (t *traversal) visit(site callSite, name string, dot types.Type, via bool) {
	key := executionKey(name, dot)
	if first, seen := t.visited[key]; seen {
		t.trimmed = append(t.trimmed, trim{
			call:     site,
			template: name,
			dataType: dot,
			firstFor: first,
		})
		return
	}
	t.visited[key] = site

	location, ok := t.index[name]
	if !ok {
		return
	}
	t.scopes = append(t.scopes, scope{
		call:         site,
		template:     name,
		dataType:     dot,
		via:          via,
		treeLocation: location,
	})

	for _, nested := range templateCalls(t.lt, location.tree, dot) {
		t.visit(site, nested.name, nested.dot, true)
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
func templateCalls(lt *checked, tree *parse.Tree, dot types.Type) []templateCall {
	var found []templateCall
	// A template that does not check still yields the invocations found
	// before the failure, which is better than none: muxt check is where
	// a type error should be reported, not here.
	_ = executeWith(lt, tree, dot, func(node *parse.TemplateNode, _ *parse.Tree, tp types.Type, _ check.Definition) {
		found = append(found, templateCall{name: node.Name, dot: tp})
	})
	return found
}

// checks reports whether tree type checks with dot, which is how a mutant
// is told from a mutation that merely breaks the template.
func checks(lt *checked, tree *parse.Tree, dot types.Type) bool {
	return executeWith(lt, tree, dot, nil) == nil
}

// executeWith type checks tree with dot, calling inspect on each
// {{template}} node it passes, then puts back whatever inspector the
// shared checker held before.
func executeWith(lt *checked, tree *parse.Tree, dot types.Type, inspect func(*parse.TemplateNode, *parse.Tree, types.Type, check.Definition)) error {
	saved := lt.global.InspectTemplateNode
	lt.global.InspectTemplateNode = inspect
	defer func() { lt.global.InspectTemplateNode = saved }()
	return check.Execute(lt.global, tree, dot)
}

// typeKey identifies a type exactly, for deciding whether a template has
// already been mutated with this dot. It keeps full package paths so that
// two same named types from different packages never collide.
//
// An alias is replaced by the type it stands for, so retargeting an alias
// -- type Page = page.Data becoming page.Model -- changes the key where the
// alias's own name would not.
func typeKey(t types.Type) string {
	if t == nil {
		return "<nil>"
	}
	return unaliased(t).String()
}

// unaliased replaces the aliases in t with what they stand for, wherever
// they sit in it: under pointers, slices, arrays, maps, channels, struct
// fields, signatures, and the type arguments of a generic type, which is
// where a route's dot keeps its data type. A type holding no alias comes
// back as it was.
func unaliased(t types.Type) types.Type {
	switch t := types.Unalias(t).(type) {
	case *types.Pointer:
		return types.NewPointer(unaliased(t.Elem()))
	case *types.Slice:
		return types.NewSlice(unaliased(t.Elem()))
	case *types.Array:
		return types.NewArray(unaliased(t.Elem()), t.Len())
	case *types.Map:
		return types.NewMap(unaliased(t.Key()), unaliased(t.Elem()))
	case *types.Chan:
		return types.NewChan(t.Dir(), unaliased(t.Elem()))
	case *types.Named:
		args := t.TypeArgs()
		if args.Len() == 0 {
			return t
		}
		unaliasedArgs := make([]types.Type, args.Len())
		for i := range unaliasedArgs {
			unaliasedArgs[i] = unaliased(args.At(i))
		}
		instance, err := types.Instantiate(nil, t.Origin(), unaliasedArgs, false)
		if err != nil {
			return t
		}
		return instance
	case *types.Struct:
		fields := make([]*types.Var, t.NumFields())
		tags := make([]string, t.NumFields())
		for i := range fields {
			f := t.Field(i)
			fields[i] = types.NewField(f.Pos(), f.Pkg(), f.Name(), unaliased(f.Type()), f.Embedded())
			tags[i] = t.Tag(i)
		}
		return types.NewStruct(fields, tags)
	case *types.Signature:
		return types.NewSignatureType(t.Recv(), nil, nil, unaliasedTuple(t.Params()), unaliasedTuple(t.Results()), t.Variadic())
	default:
		return t
	}
}

func unaliasedTuple(t *types.Tuple) *types.Tuple {
	if t == nil {
		return nil
	}
	vars := make([]*types.Var, t.Len())
	for i := range vars {
		v := t.At(i)
		vars[i] = types.NewParam(v.Pos(), v.Pkg(), v.Name(), unaliased(v.Type()))
	}
	return types.NewTuple(vars...)
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
