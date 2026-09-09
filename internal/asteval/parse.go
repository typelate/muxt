package asteval

import (
	"text/template/parse"

	"github.com/typelate/check"
)

// builtinFunctionNames are the functions text/template defines for every
// template.
//
// text/template keeps this map itself and hands it to the parser, but it
// is not exported, and a template set's Functions carries signatures only
// for the ones a signature can describe: the comparisons and the
// container builtins are checked by shape rather than by signature, so
// they are absent from it. Parsing template text therefore has to name
// them here.
//
// Only the names matter. The parser checks that a called name is known
// and never calls it.
var builtinFunctionNames = [...]string{
	"and", "call", "eq", "ge", "gt", "html", "index", "js", "le",
	"len", "lt", "ne", "not", "or", "print", "printf", "println",
	"slice", "urlquery",
}

// ParseTrees parses template text into the trees it defines, the way
// muxt's template loading parses it.
//
// Node positions in the returned trees are byte offsets into text, which
// is what lets a caller relate a node back to the source it was written
// in.
//
// functions supplies the names the template set registered beyond the
// builtins; it may be nil.
func ParseTrees(name, text, leftDelim, rightDelim string, functions check.Functions) (map[string]*parse.Tree, error) {
	return parse.Parse(name, text, leftDelim, rightDelim, TemplateFuncNames(functions))
}

// TemplateFuncNames returns the function names a template may call, in
// the shape text/template/parse wants.
func TemplateFuncNames(functions check.Functions) map[string]any {
	names := make(map[string]any, len(functions)+len(builtinFunctionNames))
	for _, name := range builtinFunctionNames {
		names[name] = nothing
	}
	for name := range functions {
		names[name] = nothing
	}
	return names
}

// nothing stands in for a function the parser will never call. It only
// checks that a name is present.
var nothing = func() string { return "" }
