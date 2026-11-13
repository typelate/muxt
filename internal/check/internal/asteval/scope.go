package asteval

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/typelate/muxt/internal/source"
	"github.com/typelate/muxt/template"
)

// Scope is the package a template chain is evaluated in. It is input only:
// TemplateMetadata stays the accumulator for what the evaluation found. It
// exists so that the evaluator takes one value for the package instead of
// seven positional parameters, and so that reading another package's
// embedded files can add a field without touching every caller.
type Scope struct {
	Types *types.Package
	Info  *types.Info
	Fset  *token.FileSet
	Files []*ast.File

	// Dir is the package directory; relative paths in errors are to it.
	Dir string

	// Embedded are the package's own embedded files, relative to Dir.
	Embedded []string

	// ModulePath is the analyzed package's module.
	ModulePath string

	// Imports are the other packages whose embedded files a ParseFS call
	// may read, by import path. Nil means no other package is readable.
	Imports map[string]source.EmbedPackage
}

// Chain is one call's state: the template set built so far and the
// delimiters in effect. It is the input and the output of one step of the
// construction chain, so that the delimiters a Delims call set reach the
// next call.
type Chain struct {
	// Templates is the set built so far, nil at the start of a chain.
	Templates *template.Template[*types.Signature]

	// LeftDelim and RightDelim are the delimiters in effect, empty for the
	// template defaults.
	LeftDelim, RightDelim string
}

// next is the return of a step that leaves the delimiters alone: the set t,
// and err if the step failed.
func (c Chain) next(t *template.Template[*types.Signature], err error) (Chain, error) {
	c.Templates = t
	return c, err
}

// delimited is the return of a Delims step: the set t with the new
// delimiters in effect.
func (c Chain) delimited(t *template.Template[*types.Signature], leftDelim, rightDelim string) (Chain, error) {
	c.Templates, c.LeftDelim, c.RightDelim = t, leftDelim, rightDelim
	return c, nil
}

// fail is the return of a step that failed. It keeps the delimiters in
// effect, so a caller that reads them after an error sees what it passed in.
func (c Chain) fail(err error) (Chain, error) {
	return c.next(nil, err)
}
