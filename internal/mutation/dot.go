package mutation

import (
	"go/types"
	"text/template/parse"

	"github.com/typelate/check"
)

// zeroLiteral returns the template literal standing for the zero value of
// the type a pipeline produces, and whether that type could be resolved.
//
// A mutation reads better, and stays closer to a real defect, when it
// substitutes a value the field could actually hold: an empty string for
// a string, zero for a count, false for a flag. Where the type cannot be
// resolved from dot the caller falls back to an empty string, which
// text/template accepts in any printing position.
func zeroLiteral(dot types.Type, pipe *parse.PipeNode, functions check.Functions) (string, bool) {
	resolved, ok := pipelineType(dot, pipe, functions)
	if !ok {
		return "", false
	}
	if isSafeString(resolved) {
		// html/template's safe string types carry a promise about their
		// contents, and a template has no way to write a literal of one:
		// "" in template source is an ordinary string, which the escaper
		// treats differently. Emptying the action is still the right
		// mutation, but calling it that type's zero value would be a
		// claim this cannot make.
		return "", false
	}
	basic, ok := resolved.Underlying().(*types.Basic)
	if !ok {
		return "", false
	}
	switch info := basic.Info(); {
	case info&types.IsString != 0:
		return `""`, true
	case info&types.IsBoolean != 0:
		return "false", true
	case info&types.IsInteger != 0, info&types.IsFloat != 0:
		return "0", true
	default:
		return "", false
	}
}

// safeStringTypes are html/template's named string types, each of which
// marks its contents as already safe for one escaping context.
var safeStringTypes = map[string]struct{}{
	"CSS": {}, "HTML": {}, "HTMLAttr": {}, "JS": {},
	"JSStr": {}, "Srcset": {}, "URL": {},
}

// isSafeString reports whether a type is one of html/template's safe
// string types.
//
// They are named types over string, so an underlying type check alone
// would take them for ordinary strings, and a function returning one is
// the usual way a project marks trusted markup.
func isSafeString(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	object := named.Obj()
	if object == nil || object.Pkg() == nil || object.Pkg().Path() != "html/template" {
		return false
	}
	_, found := safeStringTypes[object.Name()]
	return found
}

// withDot returns the type of dot inside a {{with}} body, which is what
// the with's pipeline selected.
func withDot(dot types.Type, pipe *parse.PipeNode, functions check.Functions) types.Type {
	if selected, ok := pipelineType(dot, pipe, functions); ok {
		return selected
	}
	return nil
}

// rangeDot returns the type of dot inside a {{range}} body, which is one
// element of whatever was ranged over.
//
// Ranging over an integer, which text/template has allowed since Go 1.22,
// yields the integer type itself.
func rangeDot(dot types.Type, pipe *parse.PipeNode, functions check.Functions) types.Type {
	over, ok := pipelineType(dot, pipe, functions)
	if !ok {
		return nil
	}
	switch sequence := deref(over).Underlying().(type) {
	case *types.Slice:
		return sequence.Elem()
	case *types.Array:
		return sequence.Elem()
	case *types.Map:
		return sequence.Elem()
	case *types.Chan:
		return sequence.Elem()
	case *types.Basic:
		if sequence.Info()&types.IsInteger != 0 {
			return over
		}
		return nil
	default:
		return nil
	}
}

// pipelineType resolves the type a pipeline evaluates to.
//
// A pipeline's value is whatever its last command produces, so that is
// the command resolved: for {{.Name | printf "%s"}} the answer comes
// from printf, not from .Name.
//
// A field path off dot is resolved structurally. A call is resolved from
// the function's signature, which is what the template set carries for
// every function it registered. A pipeline reading a variable is left
// alone; the fallback costs nothing but a less specific replacement.
func pipelineType(dot types.Type, pipe *parse.PipeNode, functions check.Functions) (types.Type, bool) {
	if pipe == nil || len(pipe.Cmds) == 0 {
		return nil, false
	}
	command := pipe.Cmds[len(pipe.Cmds)-1]
	if len(command.Args) == 0 {
		return nil, false
	}

	if ident, ok := command.Args[0].(*parse.IdentifierNode); ok {
		return functionResult(ident.Ident, functions)
	}
	if len(command.Args) != 1 || dot == nil {
		return nil, false
	}

	switch arg := command.Args[0].(type) {
	case *parse.DotNode:
		return dot, true
	case *parse.FieldNode:
		return fieldPath(dot, arg.Ident)
	case *parse.ChainNode:
		if _, ok := arg.Node.(*parse.DotNode); !ok {
			return nil, false
		}
		return fieldPath(dot, arg.Field)
	default:
		return nil, false
	}
}

// functionResult resolves what a called function produces.
//
// The template set carries a signature for every function it registered,
// which covers the project's own and the print and escape families. The
// builtins the checker verifies by shape rather than by signature are not
// in it, and the ones with a fixed result type are answered here.
//
// and, or, index, slice and call are deliberately unresolved: what they
// produce depends on their arguments, and guessing would put a wrong
// replacement in a mutant.
func functionResult(name string, functions check.Functions) (types.Type, bool) {
	if signature, ok := functions[name]; ok && signature.Results().Len() > 0 {
		return signature.Results().At(0).Type(), true
	}
	switch name {
	case "len":
		return types.Typ[types.Int], true
	case "eq", "ne", "lt", "le", "gt", "ge", "not":
		return types.Typ[types.Bool], true
	case "print", "printf", "println", "html", "js", "urlquery":
		return types.Typ[types.String], true
	default:
		return nil, false
	}
}

// fieldPath walks a chain of field and method names from a starting type.
func fieldPath(from types.Type, idents []string) (types.Type, bool) {
	current := from
	for _, ident := range idents {
		next, ok := memberType(current, ident)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

// memberType resolves one name against a type, as a template does: a
// struct field, or a method taking no arguments.
func memberType(from types.Type, ident string) (types.Type, bool) {
	if from == nil {
		return nil, false
	}
	if method, ok := methodType(from, ident); ok {
		return method, true
	}

	structure, ok := deref(from).Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	for i := range structure.NumFields() {
		field := structure.Field(i)
		if field.Name() == ident && field.Exported() {
			return field.Type(), true
		}
	}
	return nil, false
}

// methodType resolves a method with no parameters, whose single result,
// or whose first result alongside an error, is what a template sees.
func methodType(from types.Type, ident string) (types.Type, bool) {
	named, ok := deref(from).(*types.Named)
	if !ok {
		// A method set is also reachable through the pointer to a
		// named type, which is how most receivers are declared.
		if pointer, isPointer := from.(*types.Pointer); isPointer {
			named, ok = pointer.Elem().(*types.Named)
		}
		if !ok {
			return nil, false
		}
	}
	for i := range named.NumMethods() {
		method := named.Method(i)
		if method.Name() != ident || !method.Exported() {
			continue
		}
		signature, ok := method.Type().(*types.Signature)
		if !ok || signature.Params().Len() != 0 {
			return nil, false
		}
		results := signature.Results()
		if results.Len() == 0 {
			return nil, false
		}
		return results.At(0).Type(), true
	}
	return nil, false
}

func deref(t types.Type) types.Type {
	if pointer, ok := t.(*types.Pointer); ok {
		return pointer.Elem()
	}
	return t
}
