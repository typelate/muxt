// Package template is html/template as muxt's coverage pre-pass builds it:
// internal/overlay lays this file over $GOROOT/src/html/template, in place
// of the standard library's own files, so that a test target's
// html/template is muxt's template engine. Each type is an alias, and each
// function and constant forwards, to github.com/typelate/muxt/template/html;
// FuncMap stays text/template's, as in the standard library.
//
// It is compiled here, as a package of its own, only so that the build
// checks it; nothing imports it.
package template

import (
	"io"
	"io/fs"
	"text/template"

	h "github.com/typelate/muxt/template/html"
)

type (
	CSS       = h.CSS
	Error     = h.Error
	ErrorCode = h.ErrorCode
	FuncMap   = template.FuncMap
	HTML      = h.HTML
	HTMLAttr  = h.HTMLAttr
	JS        = h.JS
	JSStr     = h.JSStr
	Srcset    = h.Srcset
	Template  = h.Template
	URL       = h.URL
)

const (
	OK                   = h.OK
	ErrAmbigContext      = h.ErrAmbigContext
	ErrBadHTML           = h.ErrBadHTML
	ErrBranchEnd         = h.ErrBranchEnd
	ErrEndContext        = h.ErrEndContext
	ErrNoSuchTemplate    = h.ErrNoSuchTemplate
	ErrOutputContext     = h.ErrOutputContext
	ErrPartialCharset    = h.ErrPartialCharset
	ErrPartialEscape     = h.ErrPartialEscape
	ErrRangeLoopReentry  = h.ErrRangeLoopReentry
	ErrSlashAmbig        = h.ErrSlashAmbig
	ErrPredefinedEscaper = h.ErrPredefinedEscaper
	ErrJSTemplate        = h.ErrJSTemplate
)

func HTMLEscape(w io.Writer, b []byte)                  { h.HTMLEscape(w, b) }
func HTMLEscapeString(s string) string                  { return h.HTMLEscapeString(s) }
func HTMLEscaper(args ...any) string                    { return h.HTMLEscaper(args...) }
func IsTrue(val any) (truth, ok bool)                   { return h.IsTrue(val) }
func JSEscape(w io.Writer, b []byte)                    { h.JSEscape(w, b) }
func JSEscapeString(s string) string                    { return h.JSEscapeString(s) }
func JSEscaper(args ...any) string                      { return h.JSEscaper(args...) }
func URLQueryEscaper(args ...any) string                { return h.URLQueryEscaper(args...) }
func Must(t *Template, err error) *Template             { return h.Must(t, err) }
func New(name string) *Template                         { return h.New(name) }
func ParseFiles(filenames ...string) (*Template, error) { return h.ParseFiles(filenames...) }
func ParseGlob(pattern string) (*Template, error)       { return h.ParseGlob(pattern) }

func ParseFS(fs fs.FS, patterns ...string) (*Template, error) {
	return h.ParseFS(fs, patterns...)
}
