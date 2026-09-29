package muxt_test

import (
	"errors"
	"fmt"
	"go/types"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
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
func (T) AnyString(any, string) any     { return nil }
func (T) Sum(int, int) any              { return nil }
func (T) Double(int) int                { return 0 }
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
		wantErr    string
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
			wantErr:    "id is passed more than once",
		},
		{
			name:       "the first occurrence decides when it does not",
			definition: "GET /{id} StringInt(id, id)",
			param:      "id",
			wantErr:    "id is passed more than once",
		},
		{
			name:       "a nested call is walked where it is passed",
			definition: "GET /{id} Outer(Inner(id), id)",
			param:      "id",
			wantErr:    "id is passed more than once",
		},
		{
			name:       "a nested call that takes a string decides before a later int",
			definition: "GET /{id} Wrap(Echo(id), id)",
			param:      "id",
			wantErr:    "id is passed more than once",
		},
		{
			name:       "two direct occurrences of the same path value agree",
			definition: "GET /{id} AnyString(id, id)",
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
			name:       "an sse-prefixed name that is a path parameter is still a path value",
			definition: "GET /{sseID} Int(sseID)",
			param:      "sseID",
			want:       "int",
		},
		{
			name:       "a remainder wildcard parses like any parameter",
			definition: "GET /files/{path...} Int(path)",
			param:      "path",
			want:       "int",
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

			err = muxt.ResolveCall(def, srcPkg, receiver, fakeChecker)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			segment, ok := segmentByName(def.Segments, tt.param)
			require.True(t, ok, "path parameter %q not found", tt.param)
			got := pathParameterType(segment)
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
			require.NoError(t, err)
			require.NoError(t, muxt.ResolveCall(&defs[0], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, checker))
			argument := defs[0].Arguments[0]
			assert.Equal(t, tt.wantDirect, argument.Direct(), "Direct()")
			if !tt.wantDirect {
				assert.Equal(t, tt.wantMethod, argument.UnmarshalMethod(), "UnmarshalMethod()")
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
			require.NoError(t, err)
			require.NoError(t, muxt.ResolveCall(&defs[0], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, checker))
			segment, ok := segmentByName(defs[0].Segments, tt.param)
			require.True(t, ok, "path parameter %q not found", tt.param)
			assert.Equal(t, tt.want, pathParameterTextMarshaler(segment), "path parameter %q marshals as text", tt.param)
			assert.Equal(t, tt.want, defs[0].Arguments[0].TextMarshaler(), "Arguments[0].TextMarshaler()")
		})
	}
}

// TestSegmentArgument states which wildcard segments link to the resolved
// argument that first supplies their value: nil for a literal segment or a
// wildcard the call does not pass, the argument otherwise.
func TestSegmentArgument(t *testing.T) {
	pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": pathValueReceiver})
	receiver := pkg.Scope().Lookup("T").Type().(*types.Named)
	checker := fake.NewChecker().ParsesFromText(fake.Lookup(t, pkg, "Time")).Fake()

	ts := template.Must(template.New("").Parse(`{{define "GET /a/{id}/{unused} Int(id)"}}{{end}}`))
	defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: ts})
	require.NoError(t, err)
	require.NoError(t, muxt.ResolveCall(&defs[0], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, checker))
	def := defs[0]

	literal, ok := def.Segments[0], def.Segments[0].IsLiteral()
	require.True(t, ok, "the first segment is the literal %q", literal.Value())
	require.Nil(t, literal.Argument(), "a literal segment has no argument")

	idSegment, ok := segmentByName(def.Segments, "id")
	require.True(t, ok)
	require.NotNil(t, idSegment.Argument(), "id is passed to Int and should be linked")
	require.Equal(t, &def.Arguments[0], idSegment.Argument(), "the linked argument is the resolved call argument")

	unusedSegment, ok := segmentByName(def.Segments, "unused")
	require.True(t, ok)
	require.Nil(t, unusedSegment.Argument(), "unused is not passed to the call")
}

// TestArgumentDeclares states that of a repeated path value's agreeing
// occurrences, only the first depth first -- inside a nested call before the
// outer call's own argument -- declares the generator's local; later
// occurrences reuse it.
func TestArgumentDeclares(t *testing.T) {
	pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": pathValueReceiver})
	receiver := pkg.Scope().Lookup("T").Type().(*types.Named)
	checker := fake.NewChecker().ParsesFromText(fake.Lookup(t, pkg, "Time")).Fake()

	ts := template.Must(template.New("").Parse(`{{define "GET /{id} Sum(Double(id), id)"}}{{end}}`))
	defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: ts})
	require.NoError(t, err)
	require.NoError(t, muxt.ResolveCall(&defs[0], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, checker))
	def := defs[0]

	require.Equal(t, muxt.ArgumentTypeCall, def.Arguments[0].Type, "Sum's first argument is the nested Double(id) call")
	nestedID := def.Arguments[0].Arguments()[0]
	require.True(t, nestedID.Declares(), "the nested occurrence of id, reached first depth first, declares the local")

	outerID := def.Arguments[1]
	require.False(t, outerID.Declares(), "the outer occurrence of id reuses the local the nested call declared")
}

// TestSegments states how a pattern's path splits into segments and which
// wildcard spellings are rejected.
func TestSegments(t *testing.T) {
	for _, tt := range []struct {
		name       string
		definition string
		want       []string
		wantErr    string
	}{
		{name: "literals and wildcards", definition: "GET /users/{id}/files/{path...}", want: []string{"literal users", "wildcard id", "literal files", "remainder path"}},
		{name: "the end wildcard is not a segment", definition: "GET /users/{$}", want: []string{"literal users"}},
		{name: "the root has no segments", definition: "GET /"},
		{name: "a literal may repeat a wildcard name", definition: "GET /id/{id}", want: []string{"literal id", "wildcard id"}},
		{name: "an unclosed wildcard", definition: "GET /{id", wantErr: "path segment {id is not permitted"},
		{name: "a wildcard followed by text", definition: "GET /{id}x", wantErr: "path segment {id}x is not permitted"},
		{name: "an empty wildcard name", definition: "GET /{}", wantErr: `"" is not a Go identifier`},
		{name: "an empty remainder name", definition: "GET /{...}", wantErr: `"" is not a Go identifier`},
		{name: "a duplicate wildcard", definition: "GET /{id}/{id}", wantErr: `path parameter name "id" is used more than once`},
		{name: "a reserved name", definition: "GET /{form}", wantErr: "path parameter name form conflicts with a reserved identifier"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := template.Must(template.New("").Parse(fmt.Sprintf(`{{define %q}}{{end}}`, tt.definition)))
			defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: ts})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, defs, 1)
			var got []string
			for _, segment := range defs[0].Segments {
				got = append(got, describeSegment(segment))
			}
			require.Equal(t, tt.want, got)
		})
	}
}

// pathParameterType mirrors the route path generator's rule for a wildcard
// segment's helper parameter type: string when the segment names no linked
// argument or the argument is Direct, else the argument's parameter type.
func pathParameterType(segment muxt.Segment) string {
	arg := segment.Argument()
	if arg == nil || arg.Direct() {
		return "string"
	}
	return arg.ParamType().Format(func(name, _ string) string { return name })
}

// pathParameterTextMarshaler mirrors the route path generator's rule for
// whether a wildcard segment's value formats back with MarshalText.
func pathParameterTextMarshaler(segment muxt.Segment) bool {
	arg := segment.Argument()
	return arg != nil && !arg.Direct() && arg.TextMarshaler()
}

func describeSegment(segment muxt.Segment) string {
	switch {
	case segment.IsLiteral():
		return "literal " + segment.Value()
	case segment.IsRemainder():
		return "remainder " + segment.Value()
	case segment.IsWildcard():
		return "wildcard " + segment.Value()
	default:
		return "unknown " + segment.Value()
	}
}

// TestPathParameterLookup states that a wildcard segment is found among
// Segments by its Value, and a literal segment is not a path parameter.
func TestPathParameterLookup(t *testing.T) {
	ts := template.Must(template.New("").Parse(`{{define "GET /files/{id}/{path...} M(id, path)"}}{{end}}`))
	defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: ts})
	require.NoError(t, err)
	def := defs[0]

	for _, name := range []string{"id", "path"} {
		segment, ok := segmentByName(def.Segments, name)
		require.True(t, ok, "segment %q", name)
		require.Equal(t, name, segment.Value())
		require.Nil(t, segment.Argument(), "def has no resolved call")
	}
	_, ok := segmentByName(def.Segments, "files")
	require.False(t, ok, "a literal segment is not a path parameter")
}

// segmentByName finds the wildcard segment named name among segments, the
// way muxt's own unexported pathParameter does.
func segmentByName(segments []muxt.Segment, name string) (muxt.Segment, bool) {
	for _, segment := range segments {
		if segment.IsWildcard() && segment.Value() == name {
			return segment, true
		}
	}
	return muxt.Segment{}, false
}

// TestRepeatedArgumentMarksEveryUse states that the error for a repeated
// argument whose uses disagree marks each place the call passes it.
func TestRepeatedArgumentMarksEveryUse(t *testing.T) {
	for _, tt := range []struct {
		definition string
		offset     int
		also       [][2]int
	}{
		{definition: "GET /{id} StringInt(id, id)", offset: 20, also: [][2]int{{24, 26}}},
		{definition: "GET /{id} Wrap(Echo(id), id)", offset: 20, also: [][2]int{{25, 27}}},
	} {
		t.Run(tt.definition, func(t *testing.T) {
			pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": pathValueReceiver})
			receiver := pkg.Scope().Lookup("T").Type().(*types.Named)
			ts := template.Must(template.New("").Parse(fmt.Sprintf(`{{define %q}}{{end}}`, tt.definition)))
			defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: ts})
			require.NoError(t, err)

			err = muxt.ResolveCall(&defs[0], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, fake.NewChecker().Fake())

			nameErr, ok := errors.AsType[*muxt.NameError](err)
			require.True(t, ok, "ResolveCall(%q) = %v, want a *NameError", tt.definition, err)
			require.Equal(t, tt.offset, nameErr.Offset)
			require.Equal(t, 2, nameErr.Length)
			require.Equal(t, tt.also, nameErr.Also)
		})
	}
}
