package generate

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

func TestAppendParseArgumentStatementsRejectsUnresolvedCalls(t *testing.T) {
	file := scalarTestFile(t)
	for _, tt := range []struct {
		name string
		call *ast.CallExpr
		args []muxt.Argument
		want string
	}{
		{
			name: "function is not an identifier",
			call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("a"), Sel: ast.NewIdent("B")}},
			want: "expected function to be identifier",
		},
		{
			name: "arguments were not resolved",
			call: &ast.CallExpr{Fun: ast.NewIdent("F"), Args: []ast.Expr{ast.NewIdent("x")}},
			want: "call F was not resolved",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := appendParseArgumentStatements(nil, file, tt.args, "", testConfig(), tt.call, nil, nil)
			assert.EqualError(t, err, tt.want, "appendParseArgumentStatements error")
		})
	}
}

func TestAppendParseArgumentStatementsRejectsUnexpectedArgumentExpressions(t *testing.T) {
	call := &ast.CallExpr{Fun: ast.NewIdent("F"), Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}}}
	_, err := appendParseArgumentStatements(nil, scalarTestFile(t), []muxt.Argument{{}}, "", testConfig(), call, nil, nil)
	assert.EqualError(t, err, "unsupported argument 1 in call to F", "appendParseArgumentStatements error")
}

func TestAppendParseArgumentStatementsNumbersNestedCallResults(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /two Two(B(), B())"}}{{end}}`,
	})
	handler, err := callHandlerFunc(newFile(pkg), testConfig(), defs[0], "RoutesReceiver")
	require.NoError(t, err)
	got := astgen.Format(handler.Body)
	for _, want := range []string{"result0 = receiver.B()", "result1 = receiver.B()", "receiver.Two(result0, result1)"} {
		assert.Contains(t, got, want, "handler body")
	}
}

// TestNestedCallsRunOnlyWithoutErrors states that a call passed as an
// argument is guarded as the call it is an argument of is: a request that
// failed to parse must not reach the receiver. Each nesting level has a
// result of its own.
func TestNestedCallsRunOnlyWithoutErrors(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /deep Two(Nested(B()), B())"}}{{end}}`,
	})
	handler, err := callHandlerFunc(newFile(pkg), testConfig(), defs[0], "RoutesReceiver")
	require.NoError(t, err)
	got := astgen.Format(handler.Body)
	for _, want := range []string{
		"var result0 string\n\tif len(td.errList) == 0 {\n\t\tresult0 = receiver.B()\n\t}",
		"var result1 string\n\tif len(td.errList) == 0 {\n\t\tresult1 = receiver.Nested(result0)\n\t}",
		"var result2 string\n\tif len(td.errList) == 0 {\n\t\tresult2 = receiver.B()\n\t}",
		"receiver.Two(result1, result2)",
	} {
		assert.Contains(t, got, want, "handler body")
	}
	assert.NotContains(t, got, ":= receiver.", "a nested call defined inline, outside the guard")
}

// TestNestedCallResultsDoNotShadowPackageNames states that a result local
// takes a name the package does not declare: declared before a call written as
// result0(), a local named result0 would be what the call names.
func TestNestedCallResultsDoNotShadowPackageNames(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /two Two(B(), B())"}}{{end}}`,
	})
	pkg.Types.Scope().Insert(types.NewFunc(token.NoPos, pkg.Types, "result0", types.NewSignatureType(nil, nil, nil, nil, nil, false)))
	handler, err := callHandlerFunc(newFile(pkg), testConfig(), defs[0], "RoutesReceiver")
	require.NoError(t, err)
	got := astgen.Format(handler.Body)
	assert.NotContains(t, got, "result0", "a name the package declares")
	for _, want := range []string{"result1 = receiver.B()", "result2 = receiver.B()", "receiver.Two(result1, result2)"} {
		assert.Contains(t, got, want, "handler body")
	}
}

// TestRequestArgumentsAreParsedBeforeNestedCalls states that no call passed
// as an argument runs before every value of the request is read: an invalid
// value after the call must stop it.
func TestRequestArgumentsAreParsedBeforeNestedCalls(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /x/{id} Two(Nested(B()), id)"}}{{end}}`,
	})
	handler, err := callHandlerFunc(newFile(pkg), testConfig(), defs[0], "RoutesReceiver")
	require.NoError(t, err)
	got := astgen.Format(handler.Body)
	parse := strings.Index(got, `request.PathValue("id")`)
	call := strings.Index(got, "receiver.B()")
	require.NotEqual(t, -1, parse, "the path value is read")
	require.NotEqual(t, -1, call, "the nested call is made")
	assert.Less(t, parse, call, "the path value is read before the nested call")
}

func TestMismatchedArgumentError(t *testing.T) {
	err := mismatchedArgumentError(scalarTestFile(t), muxt.Argument{Identifier: "fooMessage"})
	assert.EqualError(t, err, "failed to determine type for fooMessage", "mismatchedArgumentError")
}

func TestRequestArgumentSource(t *testing.T) {
	for _, tt := range []struct {
		name     string
		argument muxt.Argument
		want     string
		wantErr  string
	}{
		{name: "body", argument: muxt.Argument{Type: muxt.ArgumentTypeRequestBody, Identifier: "body"}, want: "request.Body"},
		{name: "last event id", argument: muxt.Argument{Type: muxt.ArgumentTypeLastEventID, Identifier: "lastEventID"}, want: `request.Header.Get("Last-Event-Id")`},
		{name: "path value", argument: muxt.Argument{Type: muxt.ArgumentTypeRequestPathValue, Identifier: "id"}, want: `request.PathValue("id")`},
		{name: "unsupported", argument: muxt.Argument{Type: muxt.ArgumentTypeRequestContext, Identifier: "ctx"}, wantErr: "no request source for argument ctx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requestArgumentSource(tt.argument)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr, "requestArgumentSource(%s) error", tt.name)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, astgen.Format(got), "requestArgumentSource(%s)", tt.name)
		})
	}
}
