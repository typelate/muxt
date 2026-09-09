package mutation

import (
	"go/types"
	"text/template/parse"
)

// zeroLiteral returns the template literal standing for the zero value of
// the type a pipeline produces, and whether that type could be resolved.
//
// A mutation reads better, and stays closer to a real defect, when it
// substitutes a value the field could actually hold: an empty string for
// a string, zero for a count, false for a flag. Where the type cannot be
// resolved from dot the caller falls back to an empty string, which
// text/template accepts in any printing position.
func zeroLiteral(dot types.Type, pipe *parse.PipeNode) (string, bool) {
	resolved, ok := pipelineType(dot, pipe)
	if !ok {
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

// withDot returns the type of dot inside a {{with}} body, which is what
// the with's pipeline selected.
func withDot(dot types.Type, pipe *parse.PipeNode) types.Type {
	if selected, ok := pipelineType(dot, pipe); ok {
		return selected
	}
	return nil
}

// rangeDot returns the type of dot inside a {{range}} body, which is one
// element of whatever was ranged over.
//
// Ranging over an integer, which text/template has allowed since Go 1.22,
// yields the integer type itself.
func rangeDot(dot types.Type, pipe *parse.PipeNode) types.Type {
	over, ok := pipelineType(dot, pipe)
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

// pipelineType resolves the type a pipeline evaluates to, for the field
// access shapes a template mostly uses.
//
// Only a lone dot or a field path off dot is resolved. A pipeline calling
// a function, chaining commands, or reading a variable is left alone:
// getting those right means reimplementing the checker, and the fallback
// costs nothing but a less specific replacement.
func pipelineType(dot types.Type, pipe *parse.PipeNode) (types.Type, bool) {
	if dot == nil || pipe == nil || len(pipe.Cmds) != 1 {
		return nil, false
	}
	command := pipe.Cmds[0]
	if len(command.Args) != 1 {
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
