package muxt

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"html/template"
	"testing"

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
			if got := def.isSignalsCallback(tt.argument); got != tt.signals {
				t.Errorf("isSignalsCallback(%q) = %t, want %t", tt.argument, got, tt.signals)
			}
			if got := def.isSendMessage(tt.argument); got != tt.message {
				t.Errorf("isSendMessage(%q) = %t, want %t", tt.argument, got, tt.message)
			}
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
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveSignalsCallback(%s) error = %v, want error %t", tt.method, err, tt.wantErr)
			}
			if !tt.wantErr && (!arg.callbackHasArg || types.TypeString(arg.callbackResult, nil) != "int") {
				t.Errorf("resolveSignalsCallback(%s) recorded %v, %t, want int, true", tt.method, arg.callbackResult, arg.callbackHasArg)
			}
		})
	}
}

func TestDefinedHere(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "server.go", "package p\n\nfunc Handler() {}\n", 0)
	if err != nil {
		t.Fatal(err)
	}
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
			if got := definedHere(source.Package{Fset: tt.fset}, tt.object); got != tt.want {
				t.Errorf("definedHere() = %q, want %q", got, tt.want)
			}
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
				if err != nil {
					t.Fatalf("checkPathAndMethod() = %v, want nil", err)
				}
				return
			}
			nameError, ok := errors.AsType[*NameError](err)
			if !ok {
				t.Fatalf("checkPathAndMethod() = %v, want a *NameError", err)
			}
			if nameError.Unwrap().Error() != tt.wantErr || nameError.Offset != tt.wantOffset || nameError.Length != tt.wantLength {
				t.Errorf("checkPathAndMethod() = %q at %d+%d, want %q at %d+%d", nameError.Unwrap(), nameError.Offset, nameError.Length, tt.wantErr, tt.wantOffset, tt.wantLength)
			}
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
			if !matched {
				t.Fatalf("newDefinition(%q) did not match", tt.route)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("newDefinition(%q) error = %v, want error %t", tt.route, err, tt.wantErr)
			}
			if !tt.wantErr && def.defaultStatusCode != tt.want {
				t.Errorf("newDefinition(%q) status = %d, want %d", tt.route, def.defaultStatusCode, tt.want)
			}
		})
	}
}
