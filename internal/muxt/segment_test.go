package muxt_test

import (
	"fmt"
	"go/types"
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

// pathValueReceiver declares methods whose parameters a path value is
// passed to, one parameter type each.
const pathValueReceiver = `package server

// Time stands in for a type that parses from text.
type Time struct{}

type ID string

type T struct{}

func (T) Int(int) any                   { return nil }
func (T) String(string) any             { return nil }
func (T) Any(any) any                   { return nil }
func (T) Time(Time) any                 { return nil }
func (T) IntString(int, string) any     { return nil }
func (T) StringInt(string, int) any     { return nil }
func (T) Outer(any, string) any         { return nil }
func (T) Wrap(any, int) any             { return nil }
func (T) Echo(string) string            { return "" }
func (T) Inner(int) int                 { return 0 }
func (T) Pair(int, int) any             { return nil }
`

// TestPathValueTypes states which type a path parameter parses into: the
// parameter type of the first place the call passes it, unless a string
// can be passed there as it is, in which case it stays a string.
func TestPathValueTypes(t *testing.T) {
	for _, tt := range []struct {
		name       string
		definition string
		param      string
		want       string
	}{
		{
			name:       "parsed into an int parameter",
			definition: "GET /{id} Int(id)",
			param:      "id",
			want:       "int",
		},
		{
			name:       "a string parameter needs no parsing",
			definition: "GET /{id} String(id)",
			param:      "id",
			want:       "string",
		},
		{
			name:       "a string is assignable to any",
			definition: "GET /{id} Any(id)",
			param:      "id",
			want:       "string",
		},
		{
			name:       "a text unmarshaler",
			definition: "GET /{at} Time(at)",
			param:      "at",
			want:       "server.Time",
		},
		{
			name:       "the first occurrence decides when it parses",
			definition: "GET /{id} IntString(id, id)",
			param:      "id",
			want:       "int",
		},
		{
			name:       "the first occurrence decides when it does not",
			definition: "GET /{id} StringInt(id, id)",
			param:      "id",
			want:       "string",
		},
		{
			name:       "a nested call is walked where it is passed",
			definition: "GET /{id} Outer(Inner(id), id)",
			param:      "id",
			want:       "int",
		},
		{
			name:       "a nested call that takes a string decides before a later int",
			definition: "GET /{id} Wrap(Echo(id), id)",
			param:      "id",
			want:       "string",
		},
		{
			name:       "two parameters",
			definition: "GET /{a}/{b} Pair(a, b)",
			param:      "b",
			want:       "int",
		},
		{
			name:       "a parameter the call does not pass",
			definition: "GET /{a}/{b} Int(a)",
			param:      "b",
			want:       "string",
		},
		{
			name:       "an sse-prefixed name is a callback, not a value",
			definition: "GET /{sseID} Int(sseID)",
			param:      "sseID",
			want:       "string",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": pathValueReceiver})
			receiver := pkg.Scope().Lookup("T").Type().(*types.Named)

			ts := template.Must(template.New("").Parse(fmt.Sprintf(`{{define %q}}{{end}}`, tt.definition)))

			defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: ts})
			require.NoError(t, err)
			require.NotEmpty(t, defs)
			def := &defs[0]
			require.NotNil(t, def)

			srcPkg := source.Package{Fset: fake.FileSet, Types: pkg}
			fakeChecker := fake.NewChecker().ParsesFromText(fake.Lookup(t, pkg, "Time")).Fake()

			if err := muxt.ResolveCall(def, srcPkg, receiver, fakeChecker); err != nil {
				t.Fatal(err)
			}

			segment, ok := def.PathParameter(tt.param)
			require.True(t, ok, "path parameter %q not found", tt.param)
			got := types.TypeString(segment.Type(), (*types.Package).Name)
			require.Equal(t, tt.want, got, "wrong path parameter type")
		})
	}
}

// TestPathValueParsing states how a path value reaches its parameter: as
// the string it arrived as, or parsed by the method resolution recorded.
// Generation reads both rather than working them out again.
func TestPathValueParsing(t *testing.T) {
	pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": pathValueReceiver})
	receiver := pkg.Scope().Lookup("T").Type().(*types.Named)
	checker := fake.NewChecker().ParsesFromText(fake.Lookup(t, pkg, "Time")).Fake()
	for _, tt := range []struct {
		name       string
		template   string
		wantDirect bool
		wantMethod muxt.UnmarshalMethod
	}{
		{name: "a string parameter takes the value as it arrived", template: "GET /{id} String(id)", wantDirect: true},
		{name: "an any parameter takes the value as it arrived", template: "GET /{id} Any(id)", wantDirect: true},
		{name: "an int parameter parses", template: "GET /{id} Int(id)", wantMethod: muxt.UnmarshalInt},
		{name: "a text unmarshaler parses", template: "GET /{at} Time(at)", wantMethod: muxt.UnmarshalTextUnmarshaler},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := template.Must(template.New("").Parse(`{{define "` + tt.template + `"}}{{end}}`))
			defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: ts})
			if err != nil {
				t.Fatal(err)
			}
			if err := muxt.ResolveCall(&defs[0], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, checker); err != nil {
				t.Fatal(err)
			}
			argument := defs[0].Arguments[0]
			if got := argument.Direct(); got != tt.wantDirect {
				t.Errorf("Direct() = %t, want %t", got, tt.wantDirect)
			}
			if got := argument.UnmarshalMethod(); !tt.wantDirect && got != tt.wantMethod {
				t.Errorf("UnmarshalMethod() = %v, want %v", got, tt.wantMethod)
			}
		})
	}
}

// TestPathValueTextMarshaler states that a route path formats a path value
// with MarshalText exactly when the type it parses into is a
// TextMarshaler, as the checker says.
func TestPathValueTextMarshaler(t *testing.T) {
	pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": pathValueReceiver})
	receiver := pkg.Scope().Lookup("T").Type().(*types.Named)
	timeType := fake.Lookup(t, pkg, "Time")
	checker := fake.NewChecker().ParsesFromText(timeType).FormatsAsText(timeType).Fake()
	for _, tt := range []struct {
		template, param string
		want            bool
	}{
		{template: "GET /{at} Time(at)", param: "at", want: true},
		{template: "GET /{id} Int(id)", param: "id"},
		{template: "GET /{id} String(id)", param: "id"},
	} {
		t.Run(tt.template, func(t *testing.T) {
			ts := template.Must(template.New("").Parse(`{{define "` + tt.template + `"}}{{end}}`))
			defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: ts})
			if err != nil {
				t.Fatal(err)
			}
			if err := muxt.ResolveCall(&defs[0], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, checker); err != nil {
				t.Fatal(err)
			}
			segment, ok := defs[0].PathParameter(tt.param)
			if !ok {
				t.Fatalf("path parameter %q not found", tt.param)
			}
			if got := segment.TextMarshaler(); got != tt.want {
				t.Errorf("PathParameter(%q).TextMarshaler() = %t, want %t", tt.param, got, tt.want)
			}
		})
	}
}
