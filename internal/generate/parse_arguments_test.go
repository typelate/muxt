package generate

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"

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
			if err == nil || err.Error() != tt.want {
				t.Errorf("appendParseArgumentStatements error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAppendParseArgumentStatementsRejectsUnexpectedArgumentExpressions(t *testing.T) {
	call := &ast.CallExpr{Fun: ast.NewIdent("F"), Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}}}
	_, err := appendParseArgumentStatements(nil, scalarTestFile(t), []muxt.Argument{{}}, "", testConfig(), call, nil, nil)
	if want := "unsupported argument 1 in call to F"; err == nil || err.Error() != want {
		t.Errorf("appendParseArgumentStatements error = %v, want %q", err, want)
	}
}

func TestAppendParseArgumentStatementsNumbersNestedCallResults(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /two Two(B(), B())"}}{{end}}`,
	})
	handler, err := callHandlerFunc(newFile(pkg), testConfig(), defs[0], "RoutesReceiver")
	if err != nil {
		t.Fatal(err)
	}
	got := astgen.Format(handler.Body)
	for _, want := range []string{"result0 := receiver.B()", "result1 := receiver.B()", "receiver.Two(result0, result1)"} {
		if !strings.Contains(got, want) {
			t.Errorf("handler does not contain %q:\n%s", want, got)
		}
	}
}

func TestMismatchedArgumentError(t *testing.T) {
	err := mismatchedArgumentError(scalarTestFile(t), muxt.Argument{Identifier: "fooMessage"})
	if err == nil || err.Error() != "failed to determine type for fooMessage" {
		t.Errorf("mismatchedArgumentError = %v, want failed to determine type for fooMessage", err)
	}
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
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("requestArgumentSource(%s) error = %v, want %q", tt.name, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s := astgen.Format(got); s != tt.want {
				t.Errorf("requestArgumentSource(%s) = %q, want %q", tt.name, s, tt.want)
			}
		})
	}
}
