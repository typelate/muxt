package muxt_test

import (
	"go/types"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

func TestReservedArgumentBinding(t *testing.T) {
	pkg := exampleTypes(t)
	checker := fake.StandInChecker(t, pkg).Fake()
	receiver := fake.Lookup(t, pkg, "Server").(*types.Named)
	for _, tt := range []struct {
		name         string
		template     string
		wantType     muxt.ArgumentType
		wantDirect   bool
		wantScope    string
		wantDeclares bool
	}{
		{name: "ctx", template: `{{define "GET / Context(ctx)"}}{{end}}`, wantType: muxt.ArgumentTypeRequestContext, wantDirect: true, wantScope: "Context", wantDeclares: true},
		{name: "request", template: `{{define "GET / HTTPRequest(request)"}}{{end}}`, wantType: muxt.ArgumentTypeRequest, wantDirect: true, wantScope: "*Request"},
		{name: "response", template: `{{define "GET / HTTPResponseWriter(response)"}}{{end}}`, wantType: muxt.ArgumentTypeResponse, wantDirect: true, wantScope: "ResponseWriter"},
		{name: "form", template: `{{define "GET / URLValues(form)"}}{{end}}`, wantType: muxt.ArgumentTypeRequestForm, wantDirect: true, wantScope: "Values", wantDeclares: true},
		{name: "multipart", template: `{{define "GET / MultipartFormPtr(multipart)"}}{{end}}`, wantType: muxt.ArgumentTypeRequestMultipartForm, wantDirect: true, wantScope: "*Form", wantDeclares: true},
		{name: "body", template: `{{define "POST / Reader(body)"}}{{end}}`, wantType: muxt.ArgumentTypeRequestBody, wantDirect: true, wantScope: "Reader", wantDeclares: true},
		{name: "lastEventID", template: `{{define "GET / String(lastEventID)"}}{{end}}`, wantType: muxt.ArgumentTypeLastEventID, wantDirect: true, wantScope: "string", wantDeclares: true},
		{name: "path value", template: `{{define "GET /{id} String(id)"}}{{end}}`, wantType: muxt.ArgumentTypeRequestPathValue, wantDirect: true, wantScope: "string", wantDeclares: true},
		{name: "execute", template: `{{define "GET / ExecuteNoArg(execute)"}}{{end}}`, wantType: muxt.ArgumentTypeExecute, wantScope: "<nil>"},
		{name: "signals callback", template: `{{define "GET /b sse(StreamGoodSignals(countsSignals))"}}{{end}}`, wantType: muxt.ArgumentTypeSignalsCallback, wantScope: "<nil>"},
		{name: "sse message", template: `{{define "GET /x sse(SSECallbackNotFunc(fooMessage))"}}{{end}}{{define "fooMessage"}}{{end}}`, wantType: muxt.ArgumentTypeSendMessage, wantScope: "<nil>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defs := resolveTemplate(t, pkg, receiver, checker, tt.template)
			require.NotEmpty(t, defs, "%s resolved no definitions", tt.name)
			require.NotEmpty(t, defs[0].Arguments, "%s resolved no arguments", tt.name)
			arg := defs[0].Arguments[0]
			assert.Equal(t, tt.wantType, arg.Type, "%s Type", tt.name)
			assert.Equal(t, tt.wantDirect, arg.Direct(), "%s Direct()", tt.name)
			assert.Equal(t, tt.wantDeclares, arg.Declares(), "%s Declares()", tt.name)
			assert.Equal(t, tt.wantScope, formatType(arg.ScopeType()), "%s ScopeType()", tt.name)
		})
	}
}

func TestReservedArgumentBindingErrors(t *testing.T) {
	pkg := exampleTypes(t)
	receiver := fake.Lookup(t, pkg, "Server").(*types.Named)
	unbound := fake.NewChecker().Fake()
	for _, tt := range []struct {
		name     string
		template string
		wantErr  string
	}{
		{name: "ctx unbound", template: `{{define "GET / Context(ctx)"}}{{end}}`, wantErr: "the checker binds no type to ctx"},
		{name: "request unbound", template: `{{define "GET / HTTPRequest(request)"}}{{end}}`, wantErr: "the checker binds no type to request"},
		{name: "response unbound", template: `{{define "GET / HTTPResponseWriter(response)"}}{{end}}`, wantErr: "the checker binds no type to response"},
		{name: "body unbound", template: `{{define "POST / Reader(body)"}}{{end}}`, wantErr: "the checker binds no type to body"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: template.Must(template.New("templates").Parse(tt.template))})
			require.NoError(t, err)
			err = muxt.ResolveCall(&defs[0], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, unbound)
			assert.ErrorContains(t, err, tt.wantErr, "ResolveCall(%s)", tt.name)
		})
	}
}

func resolveTemplate(t *testing.T, pkg *types.Package, receiver *types.Named, checker muxt.Checker, text string) []muxt.Definition {
	t.Helper()
	defs, err := muxt.Definitions(source.Variable{Name: "templates", Set: template.Must(template.New("templates").Parse(text))})
	require.NoError(t, err)
	for i := range defs {
		require.NoError(t, muxt.ResolveCall(&defs[i], source.Package{Fset: fake.FileSet, Types: pkg}, receiver, checker))
	}
	return defs
}

func formatType(tp source.Type) string {
	return tp.Format(func(string, string) string { return "" })
}
