package muxt

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"html/template"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
)

func TestDefinitionRepresentationPredicates(t *testing.T) {
	wildcard := func(name string) []Segment { return []Segment{{kind: SegmentKindWildcard, value: name}} }
	for _, tt := range []struct {
		name           string
		representation Representation
		segments       []Segment
		argument       string
		signals        bool
		message        bool
	}{
		{name: "sse signals callback", representation: RepresentationSSE, argument: "countsSignals", signals: true},
		{name: "sse message", representation: RepresentationSSE, argument: "fooMessage", message: true},
		{name: "html signals callback", argument: "countsSignals"},
		{name: "html message", argument: "fooMessage"},
		{name: "json signals callback", representation: RepresentationMarshalJSON, argument: "countsSignals"},
		{name: "sse plain name", representation: RepresentationSSE, argument: "count"},
		{name: "sse path value ending in Signals", representation: RepresentationSSE, segments: wildcard("countsSignals"), argument: "countsSignals"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{Representation: tt.representation, Segments: tt.segments}
			assert.Equal(t, tt.signals, def.isSignalsCallback(tt.argument), "isSignalsCallback(%q)", tt.argument)
			assert.Equal(t, tt.message, def.isSendMessage(tt.argument), "isSendMessage(%q)", tt.argument)
		})
	}
}

func TestResolveSignalsCallback(t *testing.T) {
	pkg := checkSource(t, `package p

type Server struct{}

func (Server) Good(func(int) error) {}
func (Server) NoParams(func() error) {}
func (Server) TwoParams(func(int, int) error) {}
func (Server) NoResults(func(int)) {}
func (Server) TwoResults(func(int) (error, error)) {}
func (Server) NotFunction(int) {}
`)
	server := pkg.Scope().Lookup("Server").Type().(*types.Named)
	for _, tt := range []struct {
		method  string
		wantErr bool
	}{
		{method: "Good"},
		{method: "NoParams", wantErr: true},
		{method: "TwoParams", wantErr: true},
		{method: "NoResults", wantErr: true},
		{method: "TwoResults", wantErr: true},
		{method: "NotFunction", wantErr: true},
	} {
		t.Run(tt.method, func(t *testing.T) {
			method, _, _ := types.LookupFieldOrMethod(server, true, pkg, tt.method)
			param := method.Type().(*types.Signature).Params().At(0).Type()
			arg := &Argument{Identifier: "countsSignals", Type: ArgumentTypeSignalsCallback, paramType: param}
			err := resolveSignalsCallback(&Definition{}, arg)
			if tt.wantErr {
				require.Error(t, err, "resolveSignalsCallback(%s)", tt.method)
				return
			}
			require.NoError(t, err, "resolveSignalsCallback(%s)", tt.method)
			assert.True(t, arg.callbackHasArg, "resolveSignalsCallback(%s) callbackHasArg", tt.method)
			assert.Equal(t, "int", types.TypeString(arg.callbackResult, nil), "resolveSignalsCallback(%s) callbackResult", tt.method)
		})
	}
}

func TestDefinedHere(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "server.go", "package p\n\nfunc Handler() {}\n", 0)
	require.NoError(t, err)
	decl := file.Decls[0].(*ast.FuncDecl)
	located := types.NewFunc(decl.Name.Pos(), nil, "Handler", nil)
	unlocated := types.NewFunc(token.NoPos, nil, "Handler", nil)
	for _, tt := range []struct {
		name   string
		fset   *token.FileSet
		object types.Object
		want   string
	}{
		{name: "located", fset: fset, object: located, want: "server.go:3:6: Handler is defined here"},
		{name: "no object", fset: fset},
		{name: "no position", fset: fset, object: unlocated},
		{name: "no file set", object: located},
		{name: "position outside the file set", fset: token.NewFileSet(), object: located},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, definedHere(source.Package{Fset: tt.fset}, tt.object), "definedHere()")
		})
	}
}

func TestCheckPathAndMethod(t *testing.T) {
	for _, tt := range []struct {
		name       string
		def        Definition
		wantOffset int
		wantLength int
		wantErr    string
	}{
		{name: "valid", def: Definition{name: "GET /a/b", method: "GET", path: "/a/b", spans: nameSpans{path: [2]int{4, 8}}}},
		{name: "root", def: Definition{name: "GET /", method: "GET", path: "/", spans: nameSpans{path: [2]int{4, 5}}}},
		{name: "no method", def: Definition{name: "/a", path: "/a", spans: nameSpans{path: [2]int{0, 2}}}},
		{name: "empty segment", def: Definition{name: "GET /a//b", method: "GET", path: "/a//b", spans: nameSpans{path: [2]int{4, 9}}}, wantOffset: 7, wantLength: 1, wantErr: "path has an empty segment"},
		{name: "trailing slash", def: Definition{name: "GET /a/", method: "GET", path: "/a/", spans: nameSpans{path: [2]int{4, 7}}}, wantOffset: 6, wantLength: 1, wantErr: "path has an empty segment"},
		{name: "method not allowed", def: Definition{name: "TRACE /a", method: "TRACE", path: "/a", spans: nameSpans{method: [2]int{0, 5}, path: [2]int{6, 8}}}, wantOffset: 0, wantLength: 5, wantErr: "TRACE method not allowed; allowed methods: GET, POST, PUT, PATCH, and DELETE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.checkPathAndMethod()
			if tt.wantErr == "" {
				require.NoError(t, err, "checkPathAndMethod()")
				return
			}
			nameError, ok := errors.AsType[*NameError](err)
			require.True(t, ok, "checkPathAndMethod() = %v, want a *NameError", err)
			assert.EqualError(t, nameError.Unwrap(), tt.wantErr, "checkPathAndMethod() message")
			assert.Equal(t, tt.wantOffset, nameError.Offset, "checkPathAndMethod() offset")
			assert.Equal(t, tt.wantLength, nameError.Length, "checkPathAndMethod() length")
		})
	}
}

func TestNewDefinitionStatusCodeAndResponse(t *testing.T) {
	for _, tt := range []struct {
		name    string
		route   string
		wantErr bool
		want    int
	}{
		{name: "status without response", route: "GET / 201 Create()", want: 201},
		{name: "response without status", route: "GET / Handle(response)", want: 200},
		{name: "status with response", route: "GET / 201 Handle(response)", wantErr: true},
		{name: "named status", route: "GET / http.StatusTeapot Brew()", want: 418},
		{name: "invalid numeric status", route: "GET / 2x1 Brew()", wantErr: true},
		{name: "unknown named status", route: "GET / http.StatusNope Brew()", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def, err, matched := newDefinition(template.Must(template.New(tt.route).Parse(``)))
			require.True(t, matched, "newDefinition(%q) did not match", tt.route)
			if tt.wantErr {
				require.Error(t, err, "newDefinition(%q)", tt.route)
				return
			}
			require.NoError(t, err, "newDefinition(%q)", tt.route)
			assert.Equal(t, tt.want, def.defaultStatusCode, "newDefinition(%q) status", tt.route)
		})
	}
}

func TestWildcardName(t *testing.T) {
	for _, tt := range []struct {
		segment      string
		wantName     string
		wantWildcard bool
	}{
		{segment: "{id}", wantName: "id", wantWildcard: true},
		{segment: "{id...}", wantName: "id...", wantWildcard: true},
		{segment: "{$}", wantName: "$", wantWildcard: true},
		{segment: "{x}", wantName: "x", wantWildcard: true},
		{segment: "{}", wantName: "{}"},
		{segment: "{id", wantName: "{id"},
		{segment: "id}", wantName: "id}"},
		{segment: "users", wantName: "users"},
		{segment: "", wantName: ""},
	} {
		t.Run(tt.segment, func(t *testing.T) {
			name, isWildcard := wildcardName(tt.segment)
			assert.Equal(t, tt.wantName, name, "wildcardName(%q) name", tt.segment)
			assert.Equal(t, tt.wantWildcard, isWildcard, "wildcardName(%q) isWildcard", tt.segment)
		})
	}
}

func TestExecuteArgumentIndex(t *testing.T) {
	for _, tt := range []struct {
		name      string
		arguments []Argument
		wantIndex int
		wantOK    bool
	}{
		{name: "no arguments"},
		{name: "the execute callback", arguments: []Argument{{Type: ArgumentTypeRequestContext, Identifier: "ctx"}, {Type: ArgumentTypeExecute, Identifier: TemplateNameScopeIdentifierExecute}}, wantIndex: 1, wantOK: true},
		{name: "a render callback with another name", arguments: []Argument{{Type: ArgumentTypeExecute, Identifier: "sseClock"}}},
		{name: "the name on another kind of argument", arguments: []Argument{{Type: ArgumentTypeRequestContext, Identifier: TemplateNameScopeIdentifierExecute}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			index, ok := Definition{Arguments: tt.arguments}.ExecuteArgumentIndex()
			assert.Equal(t, tt.wantIndex, index, "ExecuteArgumentIndex() index")
			assert.Equal(t, tt.wantOK, ok, "ExecuteArgumentIndex() ok")
		})
	}
}

func TestTemplateNames(t *testing.T) {
	t.Run("no set", func(t *testing.T) {
		assert.Nil(t, templateNames(nil), "templateNames(nil)")
	})
	t.Run("every template in the set", func(t *testing.T) {
		ts := template.Must(template.New("root").Parse(`{{define "a"}}{{end}}{{define "b"}}{{end}}`))
		got := templateNames(ts)
		slices.Sort(got)
		assert.Equal(t, []string{"a", "b", "root"}, got, "templateNames()")
	})
}

func TestResolveCallNotesOnlyTheRouteCall(t *testing.T) {
	const src = `package p

type Context interface{ Done() }

type Server struct{}

func (Server) Method(Context) any { return nil }
func Function(Context) any { return nil }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.SkipObjectResolution)
	require.NoError(t, err)
	pkg, err := new(types.Config).Check("example.com/p", fset, []*ast.File{file}, nil)
	require.NoError(t, err)
	checker := scopeChecker{TemplateNameScopeIdentifierContext: pkg.Scope().Lookup("Context").Type()}
	server := pkg.Scope().Lookup("Server").Type().(*types.Named)
	for _, tt := range []struct {
		name string
		call string
		want []string
	}{
		{name: "a defined method", call: `Method(ctx)`, want: []string{"p.go:7:15: Method is defined here"}},
		{name: "nested calls add no note", call: `Method(Function(ctx))`, want: []string{"p.go:7:15: Method is defined here"}},
		{name: "a synthesized method has no position", call: `Missing(ctx)`},
		{name: "a synthesized method around a defined one", call: `Missing(Function(ctx))`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			call := mustParseCall(t, tt.call)
			def := &Definition{call: call}
			_, _, _, err := resolveCall(def, call, source.Package{Types: pkg, Fset: fset}, server, checker)
			require.NoError(t, err, "resolveCall(%s)", tt.call)
			assert.Equal(t, tt.want, def.related, "resolveCall(%s) related", tt.call)
		})
	}
}
