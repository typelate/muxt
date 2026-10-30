package check

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"strings"
)

// VerboseErrorer is implemented by errors that can render a multi-line
// description including full type or signature information.
type VerboseErrorer interface {
	error
	VerboseError() string
}

// CallError is returned when a call to a function or method fails type
// checking. It carries the signature being called and the observed argument
// types so the verbose render can show what was expected vs. what was passed.
type CallError struct {
	// Name is the function or method name. May be empty (e.g. for built-in
	// "call" or for anonymous signatures).
	Name string

	// Signature is the signature being called.
	Signature *types.Signature

	// ArgTypes are the actual argument types, in order.
	ArgTypes []types.Type

	// Cause is the short, single-line message returned by Error.
	Cause error

	// render re-renders the Cause message with a caller-chosen qualifier.
	render func(typeFormatFunc) string

	qualifier types.Qualifier
}

func (e *CallError) Error() string {
	if e == nil || e.Cause == nil {
		return ""
	}
	return e.Cause.Error()
}

func (e *CallError) Unwrap() error { return e.Cause }

// VerboseError returns a multi-line message that includes the full signature
// and the types of the arguments that were passed.
func (e *CallError) VerboseError() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(e.Error())
	if e.Signature != nil {
		b.WriteString("\n  signature: ")
		if e.Name != "" {
			b.WriteString(e.Name)
		} else {
			b.WriteString("func")
		}
		var sigBuf bytes.Buffer
		types.WriteSignature(&sigBuf, e.Signature, e.qualifier)
		b.WriteString(sigBuf.String())
	}
	if len(e.ArgTypes) > 0 {
		b.WriteString("\n  arguments:")
		for i, at := range e.ArgTypes {
			fmt.Fprintf(&b, "\n    [%d] %s", i, formatType(at, e.qualifier))
		}
	}
	return b.String()
}

// BuiltinError is returned when a call to a template built-in (len, index,
// and, or, eq, ...) is invalid. Built-ins are polymorphic and have no Go
// signature, so instead of one it carries the built-in's documented usage
// and, where relevant, a note on its semantics (e.g. and/or short-circuit
// evaluation). It renders verbose detail like CallError.
type BuiltinError struct {
	// Name is the built-in's name.
	Name string

	// Usage is the one-line usage summary of the built-in.
	Usage string

	// Note is an optional extra line of semantics (e.g. short-circuiting);
	// empty when the built-in has nothing extra worth stating.
	Note string

	// ArgTypes are the actual argument types, in order.
	ArgTypes []types.Type

	// Cause is the short, single-line message returned by Error.
	Cause error

	// render re-renders the Cause message with a caller-chosen qualifier.
	render func(typeFormatFunc) string

	qualifier types.Qualifier
}

func (e *BuiltinError) Error() string {
	if e == nil || e.Cause == nil {
		return ""
	}
	return e.Cause.Error()
}

func (e *BuiltinError) Unwrap() error { return e.Cause }

// VerboseError returns a multi-line message with the built-in's usage, the
// argument types that were passed, and any semantic note — the built-in
// analogue of CallError's signature/arguments block.
func (e *BuiltinError) VerboseError() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(e.Error())
	if e.Usage != "" {
		b.WriteString("\n  usage: ")
		b.WriteString(e.Usage)
	}
	if len(e.ArgTypes) > 0 {
		b.WriteString("\n  arguments:")
		for i, at := range e.ArgTypes {
			fmt.Fprintf(&b, "\n    [%d] %s", i, formatType(at, e.qualifier))
		}
	}
	if e.Note != "" {
		b.WriteString("\n  note: ")
		b.WriteString(e.Note)
	}
	return b.String()
}

// IdentifierError is returned when a field, method, or identifier lookup
// fails during template type checking. It carries the type that was being
// navigated so the verbose render can show its structure.
type IdentifierError struct {
	// Identifier is the field, method, or variable name being looked up.
	Identifier string

	// Type is the type the identifier was being looked up on (or the
	// signature when reporting a signature-shape problem).
	Type types.Type

	// Cause is the short, single-line message returned by Error.
	Cause error

	// render re-renders the Cause message with a caller-chosen qualifier.
	render func(typeFormatFunc) string

	qualifier types.Qualifier
	fset      *token.FileSet
}

func (e *IdentifierError) Error() string {
	if e == nil || e.Cause == nil {
		return ""
	}
	return e.Cause.Error()
}

func (e *IdentifierError) Unwrap() error { return e.Cause }

// VerboseError returns a multi-line message that, when the type is named
// and its source declaration is reachable, includes the type's Go source
// (with its godoc comment) following the short summary. For non-named
// types it falls back to printing the type and (when distinct) its
// underlying form.
func (e *IdentifierError) VerboseError() string {
	if e == nil {
		return ""
	}
	short := e.Error()

	var b strings.Builder
	b.WriteString(short)

	if src := renderTypeSource(e.Type, e.fset); src != "" {
		b.WriteString("\n\n")
		b.WriteString(src)
		return b.String()
	}

	if e.Type != nil {
		typeStr := formatType(e.Type, e.qualifier)
		b.WriteString("\n  type: ")
		b.WriteString(typeStr)
		if u := e.Type.Underlying(); u != nil && u != e.Type {
			underlyingStr := formatType(u, e.qualifier)
			if underlyingStr != typeStr {
				b.WriteString("\n  underlying: ")
				b.WriteString(underlyingStr)
			}
		}
	}
	return b.String()
}

// renderTypeSource attempts to render the Go source declaration of a
// named type, including any leading godoc comment. It reads the source
// file containing the type's declaration and prints the enclosing
// GenDecl with go/printer. Returns "" when the type is not named, has
// no valid declaration position, or the file cannot be read or parsed.
func renderTypeSource(t types.Type, fset *token.FileSet) string {
	if t == nil || fset == nil {
		return ""
	}
	named, ok := t.(*types.Named)
	if !ok {
		return ""
	}
	obj := named.Obj()
	if obj == nil {
		return ""
	}
	pos := obj.Pos()
	if !pos.IsValid() {
		return ""
	}
	tokFile := fset.File(pos)
	if tokFile == nil {
		return ""
	}

	src, err := os.ReadFile(tokFile.Name())
	if err != nil {
		return ""
	}
	parseFset := token.NewFileSet()
	parsed, err := parser.ParseFile(parseFset, tokFile.Name(), src, parser.ParseComments)
	if err != nil {
		return ""
	}

	target := obj.Name()
	for _, decl := range parsed.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name == nil || ts.Name.Name != target {
				continue
			}
			single := *gd
			single.Specs = []ast.Spec{ts}
			single.Lparen = token.NoPos
			single.Rparen = token.NoPos
			if single.Doc == nil && ts.Doc != nil {
				single.Doc = ts.Doc
			}
			var buf bytes.Buffer
			if err := printer.Fprint(&buf, parseFset, &single); err != nil {
				return ""
			}
			return strings.TrimRight(buf.String(), "\n")
		}
	}
	return ""
}

func formatType(t types.Type, qf types.Qualifier) string {
	if t == nil {
		return "<nil>"
	}
	var buf bytes.Buffer
	types.WriteType(&buf, t, qf)
	return buf.String()
}

// FormatVerbose renders err in verbose form. For each leaf in an
// errors.Join tree (or the err itself when not joined), FormatVerbose
// prefers a leaf's VerboseError method when available and falls back to
// Error otherwise. Multiple verbose blocks are separated by a blank line.
//
// FormatVerbose returns err.Error() unchanged when no error in the tree
// implements VerboseErrorer.
func FormatVerbose(err error) string {
	if err == nil {
		return ""
	}
	leaves := flattenJoined(err)
	parts := make([]string, len(leaves))
	var anyVerbose bool
	for i, leaf := range leaves {
		if v, ok := errors.AsType[VerboseErrorer](leaf); ok {
			parts[i] = v.VerboseError()
			anyVerbose = true
		} else {
			parts[i] = leaf.Error()
		}
	}
	if !anyVerbose {
		return err.Error()
	}
	return strings.Join(parts, "\n\n")
}

// flattenJoined walks errors.Join trees (errors that expose Unwrap() []error)
// and returns each non-joined leaf in the order they appear. Errors that do
// not multi-unwrap are returned as-is.
func flattenJoined(err error) []error {
	// A leaf *Error multi-unwraps to expose its cause, but renders as a
	// single verbose block; only aggregates flatten into their children.
	if e, ok := err.(*Error); ok {
		if len(e.children) == 0 {
			return []error{e}
		}
		var out []error
		for _, child := range e.children {
			out = append(out, flattenJoined(child)...)
		}
		return out
	}
	type multi interface{ Unwrap() []error }
	if m, ok := err.(multi); ok {
		var out []error
		for _, e := range m.Unwrap() {
			if e == nil {
				continue
			}
			out = append(out, flattenJoined(e)...)
		}
		return out
	}
	return []error{err}
}
