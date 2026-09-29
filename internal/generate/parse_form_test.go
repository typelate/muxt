package generate

import (
	"go/ast"
	"go/types"
	"testing"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/source"
)

func TestTypedVar(t *testing.T) {
	tp := source.NewType(types.NewMap(types.Typ[types.String], types.NewSlice(types.Typ[types.String])))
	for _, tt := range []struct {
		name  string
		value ast.Expr
		want  string
	}{
		{name: "without a value", want: "var form map[string][]string"},
		{name: "form", value: requestField("Form"), want: "var form map[string][]string = request.Form"},
		{name: "multipart form", value: requestField("MultipartForm"), want: "var form map[string][]string = request.MultipartForm"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decl, err := typedVar(scalarTestFile(t), "form", tp, tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if got := astgen.Format(decl); got != tt.want {
				t.Errorf("typedVar(form, %s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
