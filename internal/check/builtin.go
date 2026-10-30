package check

import (
	"errors"
	"go/types"
)

// Builtin documents a template built-in function. The check package uses it
// to render helpful errors when a built-in is misused, and tooling (an LSP
// hover, say) uses it to describe a built-in under the cursor.
type Builtin struct {
	Name  string // the built-in's name
	Usage string // one-line usage summary
	Note  string // optional extra semantics (e.g. short-circuiting); "" when none
}

// builtinDocs describes the built-ins that check evaluates structurally
// (those handled by builtInCheck). print/printf/println/html/js/urlquery are
// resolved as real Go signatures via DefaultFunctions, not here, so they are
// intentionally absent. Wording follows the text/template package docs.
var builtinDocs = map[string]Builtin{
	"and": {
		Name:  "and",
		Usage: "and arg1 arg2 [arg3 ...] — yields the first empty (falsy) argument, or the last argument if none are empty",
		Note:  "Short-circuits: arguments are evaluated left to right and evaluation stops at the first empty one, so later arguments (and their side effects) may not run.",
	},
	"or": {
		Name:  "or",
		Usage: "or arg1 arg2 [arg3 ...] — yields the first non-empty (truthy) argument, or the last argument if all are empty",
		Note:  "Short-circuits: arguments are evaluated left to right and evaluation stops at the first non-empty one, so later arguments (and their side effects) may not run.",
	},
	"not": {
		Name:  "not",
		Usage: "not arg — returns true when arg is empty (falsy), false otherwise",
	},
	"len": {
		Name:  "len",
		Usage: "len arg — the length of a string, slice, array, or map",
	},
	"index": {
		Name:  "index",
		Usage: "index collection key1 [key2 ...] — the element at the given keys; index x 1 2 is x[1][2]",
		Note:  "Each indexed value must be a slice, array, or map, applied left to right.",
	},
	"slice": {
		Name:  "slice",
		Usage: "slice arg [i [j [k]]] — a subslice: slice x is x[:], slice x 1 is x[1:], slice x 1 2 is x[1:2], slice x 1 2 3 is x[1:2:3]",
		Note:  "arg must be a string, slice, or array; a string cannot take the three-index (capacity) form.",
	},
	"call": {
		Name:  "call",
		Usage: "call fn [arg1 ...] — calls fn (a function value) with the given arguments and returns its result",
	},
	"eq": {
		Name:  "eq",
		Usage: "eq arg1 arg2 [arg3 ...] — reports whether arg1 == arg2 (with more arguments, arg1 == arg2 || arg1 == arg3 || ...)",
	},
	"ne": {Name: "ne", Usage: "ne arg1 arg2 — reports whether arg1 != arg2"},
	"lt": {Name: "lt", Usage: "lt arg1 arg2 — reports whether arg1 < arg2"},
	"le": {Name: "le", Usage: "le arg1 arg2 — reports whether arg1 <= arg2"},
	"gt": {Name: "gt", Usage: "gt arg1 arg2 — reports whether arg1 > arg2"},
	"ge": {Name: "ge", Usage: "ge arg1 arg2 — reports whether arg1 >= arg2"},
}

// BuiltinDoc returns documentation for a template built-in by name. The
// second result is false when name is not a documented built-in.
func BuiltinDoc(name string) (Builtin, bool) {
	b, ok := builtinDocs[name]
	return b, ok
}

// builtinErr builds a call-arguments *Error wrapping a *BuiltinError for
// the named built-in, filling its usage and note from builtinDocs so every
// built-in error carries the same helpful detail. types.Type args are
// rendered with a nil qualifier in the message and re-rendered with the
// caller's qualifier by DetailedError.
func (g *Global) builtinErr(name string, argTypes []types.Type, format string, args ...any) *Error {
	doc := builtinDocs[name]
	render := renderer(format, args...)
	return wrapError(ErrorTypeCallArguments, nil, nil, &BuiltinError{
		Name:      name,
		Usage:     doc.Usage,
		Note:      doc.Note,
		ArgTypes:  argTypes,
		Cause:     errors.New(render(fullTypeFormat(nil))),
		render:    render,
		qualifier: g.Qualifier,
	})
}
