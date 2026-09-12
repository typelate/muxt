package muxt_test

import (
	"go/types"
	"html/template"
	"testing"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/typestest"
)

// pathValueReceiver declares methods whose parameters a path value is
// passed to, one parameter type each.
const pathValueReceiver = `package server

import "time"

type ID string

type T struct{}

func (T) Int(int) any                   { return nil }
func (T) String(string) any             { return nil }
func (T) Any(any) any                   { return nil }
func (T) Time(time.Time) any            { return nil }
func (T) IntString(int, string) any     { return nil }
func (T) StringInt(string, int) any     { return nil }
func (T) Outer(any, int) any            { return nil }
func (T) Inner(int) int                 { return 0 }
func (T) Pair(int, int) any             { return nil }
`

// TestPathValueTypes states which type a path parameter parses into: the
// parameter type of the first place the call passes it, unless a string
// can be passed there as it is.
func TestPathValueTypes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		template string
		param    string
		want     string // "" when the parameter stays the string it arrived as
	}{
		{name: "parsed into an int parameter", template: "GET /{id} Int(id)", param: "id", want: "int"},
		{name: "a string parameter needs no parsing", template: "GET /{id} String(id)", param: "id"},
		{name: "a string is assignable to any", template: "GET /{id} Any(id)", param: "id"},
		{name: "a text unmarshaler", template: "GET /{at} Time(at)", param: "at", want: "time.Time"},
		{name: "the first occurrence decides when it parses", template: "GET /{id} IntString(id, id)", param: "id", want: "int"},
		{name: "the first occurrence decides when it does not", template: "GET /{id} StringInt(id, id)", param: "id"},
		{name: "a nested call is walked where it is passed", template: "GET /{id} Outer(Inner(id), id)", param: "id", want: "int"},
		{name: "two parameters", template: "GET /{a}/{b} Pair(a, b)", param: "b", want: "int"},
		{name: "a parameter the call does not pass", template: "GET /{a}/{b} Int(a)", param: "b"},
		{name: "an sse-prefixed name is a callback, not a value", template: "GET /{sseID} Int(sseID)", param: "sseID"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pkg := typestest.MustCheck(t, "example.com/server", pathValueReceiver)
			receiver := pkg.Scope().Lookup("T").Type().(*types.Named)
			ts := template.Must(template.New("").Parse(`{{define "` + tt.template + `"}}{{end}}`))
			defs, err := muxt.Definitions(muxt.Templates{Variable: "templates", Set: ts})
			if err != nil {
				t.Fatal(err)
			}
			if err := muxt.ResolveCall(&defs[0], muxt.Package{Fset: typestest.FileSet, Types: pkg, Lookup: typestest.Lookup}, receiver); err != nil {
				t.Fatal(err)
			}
			tp, ok := defs[0].ArgumentType(tt.param)
			got := ""
			if ok {
				got = types.TypeString(tp, (*types.Package).Name)
			}
			if got != tt.want {
				t.Errorf("ArgumentType(%q) = %q, want %q", tt.param, got, tt.want)
			}
		})
	}
}
