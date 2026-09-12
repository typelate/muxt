package typestest

// stubs maps an import path to the source of its stub package. The
// declarations follow the standard library's; bodies return zero values.
var stubs = map[string]string{
	"context": `package context

import "time"

type Context interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}

func Background() Context { return nil }
`,

	"encoding": `package encoding

type TextMarshaler interface {
	MarshalText() (text []byte, err error)
}

type TextUnmarshaler interface {
	UnmarshalText(text []byte) error
}
`,

	"encoding/json": `package json

type RawMessage []byte

func (m RawMessage) MarshalJSON() ([]byte, error) { return nil, nil }

func (m *RawMessage) UnmarshalJSON(data []byte) error { return nil }

func Marshal(v any) ([]byte, error) { return nil, nil }

func Unmarshal(data []byte, v any) error { return nil }
`,

	"errors": `package errors

func New(text string) error { return nil }

func Join(errs ...error) error { return nil }
`,

	"fmt": `package fmt

type Stringer interface {
	String() string
}

func Sprint(a ...any) string { return "" }

func Sprintf(format string, a ...any) string { return "" }

func Sprintln(a ...any) string { return "" }

func Errorf(format string, a ...any) error { return nil }
`,

	"embed": `package embed

import "io/fs"

type FS struct{}

func (f FS) Open(name string) (fs.File, error) { return nil, nil }
`,

	"html/template": `package template

import (
	"fmt"
	"io"
	"io/fs"
)

type Template struct{}

type FuncMap map[string]any

func New(name string) *Template { return nil }

func Must(t *Template, err error) *Template { return t }

func ParseFS(fsys fs.FS, patterns ...string) (*Template, error) { return nil, nil }

func (t *Template) New(name string) *Template { return t }

func (t *Template) Parse(text string) (*Template, error) { return t, nil }

func (t *Template) ParseFS(fsys fs.FS, patterns ...string) (*Template, error) { return t, nil }

func (t *Template) Funcs(funcMap FuncMap) *Template { return t }

func (t *Template) Delims(left, right string) *Template { return t }

func (t *Template) Option(opt ...string) *Template { return t }

func (t *Template) ExecuteTemplate(wr io.Writer, name string, data any) error { return nil }

func HTMLEscaper(args ...any) string { return "" }

func JSEscaper(args ...any) string { return "" }

func URLQueryEscaper(args ...any) string { return "" }

// The real package reaches fmt through its imports; check.DefaultFunctions
// finds print, printf and println there.
var _ fmt.Stringer
`,

	"io/fs": `package fs

type FileInfo interface {
	Name() string
	Size() int64
	IsDir() bool
}

type File interface {
	Stat() (FileInfo, error)
	Read([]byte) (int, error)
	Close() error
}

type FS interface {
	Open(name string) (File, error)
}
`,

	"io": `package io

type Reader interface {
	Read(p []byte) (n int, err error)
}

type Writer interface {
	Write(p []byte) (n int, err error)
}

type Closer interface {
	Close() error
}

type ReadCloser interface {
	Reader
	Closer
}

func WriteString(w Writer, s string) (n int, err error) { return 0, nil }
`,

	"mime/multipart": `package multipart

import "net/textproto"

type File interface {
	Read(p []byte) (n int, err error)
	Close() error
}

type FileHeader struct {
	Filename string
	Header   textproto.MIMEHeader
	Size     int64
}

func (fh *FileHeader) Open() (File, error) { return nil, nil }

type Form struct {
	Value map[string][]string
	File  map[string][]*FileHeader
}
`,

	"net/http": `package http

import (
	"context"
	"io"
	"mime/multipart"
	"net/url"
)

type Header map[string][]string

func (h Header) Get(key string) string { return "" }

func (h Header) Set(key, value string) {}

type Request struct {
	Method        string
	URL           *url.URL
	Header        Header
	Body          io.ReadCloser
	Form          url.Values
	PostForm      url.Values
	MultipartForm *multipart.Form
	Pattern       string
}

func (r *Request) Context() context.Context { return nil }

func (r *Request) PathValue(name string) string { return "" }

func (r *Request) FormValue(key string) string { return "" }

func (r *Request) ParseForm() error { return nil }

func (r *Request) ParseMultipartForm(maxMemory int64) error { return nil }

type ResponseWriter interface {
	Header() Header
	Write([]byte) (int, error)
	WriteHeader(statusCode int)
}

type Flusher interface {
	Flush()
}

type Handler interface {
	ServeHTTP(ResponseWriter, *Request)
}

type HandlerFunc func(ResponseWriter, *Request)

func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) {}

type ServeMux struct{}

func (mux *ServeMux) Handle(pattern string, handler Handler) {}

func (mux *ServeMux) HandleFunc(pattern string, handler func(ResponseWriter, *Request)) {}
`,

	"net/textproto": `package textproto

type MIMEHeader map[string][]string
`,

	"net/url": `package url

type URL struct {
	Scheme   string
	Host     string
	Path     string
	RawQuery string
}

type Values map[string][]string

func (v Values) Get(key string) string { return "" }

func PathEscape(s string) string { return "" }
`,

	"time": `package time

type Duration int64

type Time struct {
	wall uint64
	ext  int64
}

func (t Time) MarshalText() ([]byte, error) { return nil, nil }

func (t *Time) UnmarshalText(data []byte) error { return nil }

func Now() Time { return Time{} }
`,
}
