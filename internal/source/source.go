// Package source holds a Go package as muxt reads it: its types, and each
// templates variable it declares with where its templates were written and
// where they are executed.
//
// It holds nothing about the standard library. What route resolution needs
// to know about that is asked of a muxt.Checker.
//
// internal/load builds a Package from a go/packages load; that is the only
// step that runs the go command. Everything muxt does after it -- route
// resolution, generation, type checking templates -- reads a Package. It
// is plain data, with no function fields and no loader behind it, so a
// test can write one as a literal or build one from source type checked in
// memory.
package source

import (
	"go/token"
	"go/types"
	"html/template"
	"text/template/parse"
)

// Package is a loaded Go package.
type Package struct {
	// Fset positions every object in Types and every position in the
	// variables.
	Fset *token.FileSet

	// Types is the package: the one declaring the templates variables,
	// whose package-scope functions a template name may call.
	Types *types.Package

	// Variables are the templates variables that were asked for, in the
	// order they were named.
	Variables []Variable
}

// Variable is one package-level templates variable.
type Variable struct {
	// Name is the variable's name.
	Name string

	// Set is the template set the variable holds. A text/template
	// variable is carried as an html/template value with the same trees:
	// muxt reads names and trees and never executes the set.
	Set *template.Template

	// Functions are the functions a template in the set may call: the
	// builtins and those registered with Funcs.
	Functions map[string]*types.Signature

	// Funcs are only the functions registered with Funcs calls in the
	// variable's construction chain.
	Funcs map[string]*types.Signature

	// Definitions locate where each template in the set was written, by
	// template name. A template whose source is unknown has none.
	Definitions map[string]Definition

	// Calls are the variable's ExecuteTemplate calls with a string
	// literal template name, in file order.
	Calls []Call
}

// NamePosition reports where a template's name begins in the source that
// defines it: the first byte inside the name's quotes.
func (v Variable) NamePosition(templateName string) (token.Position, bool) {
	definition, ok := v.Definitions[templateName]
	if !ok || !definition.TemplateName.IsValid() {
		return token.Position{}, false
	}
	pos := definition.TemplateName.Position
	pos.Column++
	pos.Offset++
	return pos, true
}

// Definition locates the text defining one template.
type Definition struct {
	// Name is the template's name.
	Name string

	// Define spans the define or block clause, End the clause that closes
	// it, and TemplateName the quoted name in the define clause. For a
	// template with no define clause -- the one a parsed text itself
	// carries -- TemplateName and End are unset and Define spans the text.
	Define, End, TemplateName Span

	// Tree is the template's parse tree.
	Tree *parse.Tree
}

// Span is a run of bytes in a file.
type Span struct {
	token.Position
	Length int
}

// Call is one templatesVariable.ExecuteTemplate(w, name, data) call.
type Call struct {
	// Position is where the call is written.
	Position token.Position

	// Template is the template name the call passes.
	Template string

	// Data is the type of the data argument, the type of dot the
	// template is executed with.
	Data types.Type
}
