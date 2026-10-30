package check

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxInlineTypeLength bounds how long an inline (unnamed) type may render in
// DetailedError output before it is elided to a summary such as struct{...}.
const maxInlineTypeLength = 64

// DetailedError writes a multi-line rendering of the error tree to w. Each
// failure block starts with the compact single-line message, followed by
// supporting detail: identifier lookup failures render the receiver type as
// an indented Go-style declaration — exported fields (private fields are
// noted but omitted) followed by exported methods as func declarations that
// keep the declared receiver, so pointer receivers stay visible; call
// failures show the callee signature and the argument types.
//
// Every line — including the leading location line — prints type names with
// types.WriteType using q, so a caller can qualify packages for the target
// file's import context, for example a language server rendering types with
// the identifiers the importing file declares. A nil q prints full package
// paths, matching Error. The rendered type declaration is the exception: it
// reads as source at its declaration site, so types from the declared
// type's own package print bare while foreign packages still use q (or
// their full path when q is nil).
//
// Unlike Error, massive inline composite types are elided: an unnamed
// struct, interface, or signature whose rendering exceeds a small threshold
// prints as struct{...}, interface{...}, or a signature with unnamed
// shortened parameters, with container shape preserved (e.g.
// []struct{...}). The full types remain available on the Error's X field
// and the IdentifierError/CallError causes.
//
// DetailedError returns the first error encountered writing to w, or nil.
func (e *Error) DetailedError(w io.Writer, q types.Qualifier) error {
	sw := &stickyWriter{w: w}
	e.writeDetail(sw, q, shortTypeFormat(q))
	return sw.err
}

func (e *Error) writeDetail(sw *stickyWriter, q types.Qualifier, tf typeFormatFunc) {
	if len(e.children) > 0 {
		for i, child := range e.children {
			if i > 0 {
				sw.writeString("\n\n")
			}
			child.writeDetail(sw, q, tf)
		}
		return
	}
	sw.writeString(e.line(tf))
	if identErr, ok := errors.AsType[*IdentifierError](e.err); ok && identErr.Type != nil {
		sw.writeString("\n\n")
		writeTypeDecl(sw, identErr.Type, declTypeFormat(identErr.Type, q, tf))
		return
	}
	if callErr, ok := errors.AsType[*CallError](e.err); ok && callErr.Signature != nil {
		writeCallDetail(sw, callErr, tf)
	}
}

// stickyWriter forwards writes to w until one fails, then drops the rest and
// keeps the first error.
type stickyWriter struct {
	w   io.Writer
	err error
}

func (sw *stickyWriter) writeString(s string) {
	if sw.err != nil {
		return
	}
	_, sw.err = io.WriteString(sw.w, s)
}

func (sw *stickyWriter) writef(format string, args ...any) {
	if sw.err != nil {
		return
	}
	_, sw.err = fmt.Fprintf(sw.w, format, args...)
}

// shortTypeFormat renders types qualified with q, eliding unnamed composite
// types whose full rendering exceeds maxInlineTypeLength. Container shape
// is preserved so []struct{...} and map[string]struct{...} stay
// recognizable; named types always render in full (their names are short).
func shortTypeFormat(q types.Qualifier) typeFormatFunc {
	var short typeFormatFunc
	short = func(tp types.Type) string {
		full := formatType(tp, q)
		if len(full) <= maxInlineTypeLength {
			return full
		}
		switch t := tp.(type) {
		case *types.Pointer:
			return "*" + short(t.Elem())
		case *types.Slice:
			return "[]" + short(t.Elem())
		case *types.Array:
			return fmt.Sprintf("[%d]%s", t.Len(), short(t.Elem()))
		case *types.Map:
			return "map[" + short(t.Key()) + "]" + short(t.Elem())
		case *types.Chan:
			switch t.Dir() {
			case types.SendOnly:
				return "chan<- " + short(t.Elem())
			case types.RecvOnly:
				return "<-chan " + short(t.Elem())
			default:
				return "chan " + short(t.Elem())
			}
		case *types.Struct:
			return "struct{...}"
		case *types.Interface:
			return "interface{...}"
		case *types.Signature:
			return shortSignature(t, short)
		default:
			return full
		}
	}
	return short
}

// shortSignature renders a signature with shortened, unnamed parameter and
// result types.
func shortSignature(sig *types.Signature, short typeFormatFunc) string {
	var b strings.Builder
	b.WriteString("func(")
	params := sig.Params()
	for i := range params.Len() {
		if i > 0 {
			b.WriteString(", ")
		}
		pt := params.At(i).Type()
		if sig.Variadic() && i == params.Len()-1 {
			if s, ok := pt.(*types.Slice); ok {
				b.WriteString("...")
				b.WriteString(short(s.Elem()))
				continue
			}
		}
		b.WriteString(short(pt))
	}
	b.WriteString(")")
	results := sig.Results()
	switch results.Len() {
	case 0:
	case 1:
		b.WriteString(" ")
		b.WriteString(short(results.At(0).Type()))
	default:
		b.WriteString(" (")
		for i := range results.Len() {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(short(results.At(i).Type()))
		}
		b.WriteString(")")
	}
	return b.String()
}

// writeTypeDecl writes tp as an indented Go-style type declaration: the
// exported fields inside a struct (or interface) body, followed by the
// exported methods rendered as func declarations with their declared
// receivers, so pointer receivers stay visible. A type with no exported
// members gets "T has no exported fields or methods".
func writeTypeDecl(sw *stickyWriter, tp types.Type, tf typeFormatFunc) {
	fields, methods, unexportedFields := typeMembers(tp, tf)
	if len(fields) == 0 && len(methods) == 0 {
		sw.writeString(tf(tp))
		sw.writeString(" has no exported fields or methods")
		return
	}
	name := declName(tp)
	switch tp.Underlying().(type) {
	case *types.Interface:
		if name != "" {
			sw.writef("  type %s interface {", name)
		} else {
			sw.writeString("  interface {")
		}
		for _, m := range methods {
			sw.writef("\n    %s%s", m.name, m.detail)
		}
		sw.writeString("\n  }")
		return
	case *types.Struct:
		if name != "" {
			sw.writef("  type %s struct", name)
		} else {
			sw.writeString("  struct")
		}
		if len(fields) == 0 && !unexportedFields {
			sw.writeString("{}")
			break
		}
		sw.writeString(" {")
		width := 0
		for _, f := range fields {
			width = max(width, len(f.name))
		}
		for _, f := range fields {
			sw.writef("\n    %-*s %s", width, f.name, f.detail)
		}
		if unexportedFields {
			sw.writeString("\n    // private fields omitted")
		}
		sw.writeString("\n  }")
	default:
		sw.writef("  type %s %s", name, tf(tp.Underlying()))
	}
	if len(methods) > 0 {
		sw.writeString("\n")
		for _, m := range methods {
			sw.writef("\n  func (%s) %s%s { /* ... */ }", m.recv, m.name, m.detail)
		}
	}
}

// declTypeFormat renders types as they read in source at tp's declaration
// site: types from tp's own package print bare, foreign packages use q (or
// their full path when q is nil). Unnamed types have no declaration site,
// so tf is returned unchanged.
func declTypeFormat(tp types.Type, q types.Qualifier, tf typeFormatFunc) typeFormatFunc {
	var local *types.Package
	switch t := dereference(tp).(type) {
	case *types.Named:
		local = t.Obj().Pkg()
	case *types.Alias:
		local = t.Obj().Pkg()
	}
	if local == nil {
		return tf
	}
	return shortTypeFormat(func(p *types.Package) string {
		if p == local {
			return ""
		}
		if q == nil {
			return p.Path()
		}
		return q(p)
	})
}

func writeCallDetail(sw *stickyWriter, callErr *CallError, tf typeFormatFunc) {
	sw.writeString("\n\n  signature: ")
	if callErr.Name != "" {
		sw.writeString(callErr.Name)
	} else {
		sw.writeString("func")
	}
	sw.writeString(strings.TrimPrefix(tf(callErr.Signature), "func"))
	if len(callErr.ArgTypes) > 0 {
		sw.writeString("\n  arguments:")
		for i, at := range callErr.ArgTypes {
			sw.writef("\n    [%d] %s", i, tf(at))
		}
	}
}

// typeMember pairs a field or method name with its rendered type or
// signature. For methods on concrete types, recv holds the rendered declared
// receiver (for example "p *web.Page"); it is empty for interface methods.
type typeMember struct {
	name, detail string
	recv         string
}

// typeMembers collects tp's exported struct fields (including promoted
// fields from embedded structs) and the exported methods in its method set,
// rendered with tf. unexportedFields reports whether any unexported field
// was skipped along the way.
func typeMembers(tp types.Type, tf typeFormatFunc) (fields, methods []typeMember, unexportedFields bool) {
	seen := make(map[string]bool)

	// For interfaces, use the type directly (pointer-to-interface has an
	// empty method set). For concrete types, use the pointer to pick up
	// pointer receiver methods.
	var mset *types.MethodSet
	if types.IsInterface(tp) {
		mset = types.NewMethodSet(tp)
	} else {
		mset = types.NewMethodSet(types.NewPointer(tp))
	}
	for sel := range mset.Methods() {
		obj := sel.Obj()
		name := obj.Name()
		if !token.IsExported(name) || seen[name] {
			continue
		}
		seen[name] = true
		recv := ""
		if sig, ok := obj.Type().(*types.Signature); ok && !types.IsInterface(tp) && sig.Recv() != nil {
			recv = formatReceiver(sig.Recv(), tf)
		}
		methods = append(methods, typeMember{
			name:   name,
			detail: strings.TrimPrefix(tf(obj.Type()), "func"),
			recv:   recv,
		})
	}

	if st, ok := tp.Underlying().(*types.Struct); ok {
		unexportedFields = collectFieldMembers(st, tf, seen, &fields)
	}
	return fields, methods, unexportedFields
}

// formatReceiver renders a method receiver as it would appear in a func
// declaration: the receiver type is bare (no package selector), as at its
// declaration site. A receiver declared without a name gets one derived
// from the first letter of its type name so the declaration reads
// naturally.
func formatReceiver(r *types.Var, tf typeFormatFunc) string {
	name := r.Name()
	if name == "" || name == "_" {
		name = receiverLetter(r.Type())
	}
	return name + " " + receiverTypeName(r.Type(), tf)
}

// declName returns tp's bare declared name, or "" for unnamed types.
func declName(tp types.Type) string {
	switch t := tp.(type) {
	case *types.Named:
		return t.Obj().Name()
	case *types.Alias:
		return t.Obj().Name()
	}
	return ""
}

// receiverTypeName renders a receiver type bare, keeping a leading * for
// pointer receivers.
func receiverTypeName(tp types.Type, tf typeFormatFunc) string {
	if p, ok := tp.(*types.Pointer); ok {
		return "*" + receiverTypeName(p.Elem(), tf)
	}
	if name := declName(tp); name != "" {
		return name
	}
	return tf(tp)
}

// receiverLetter derives a one-letter receiver name from the receiver's
// (possibly pointer) named type.
func receiverLetter(tp types.Type) string {
	if p, ok := tp.(*types.Pointer); ok {
		tp = p.Elem()
	}
	if n, ok := tp.(*types.Named); ok && n.Obj().Name() != "" {
		r, _ := utf8.DecodeRuneInString(n.Obj().Name())
		return string(unicode.ToLower(r))
	}
	return "r"
}

// collectFieldMembers reports whether it skipped any unexported field.
func collectFieldMembers(st *types.Struct, tf typeFormatFunc, seen map[string]bool, fields *[]typeMember) (unexported bool) {
	for f := range st.Fields() {
		if !f.Exported() {
			unexported = true
			continue
		}
		if f.Embedded() {
			// Dereference so fields promoted through an embedded *T are
			// listed too.
			if inner, ok := dereference(f.Type()).Underlying().(*types.Struct); ok {
				if collectFieldMembers(inner, tf, seen, fields) {
					unexported = true
				}
			}
			continue
		}
		if seen[f.Name()] {
			continue
		}
		seen[f.Name()] = true
		*fields = append(*fields, typeMember{name: f.Name(), detail: tf(f.Type())})
	}
	return unexported
}
