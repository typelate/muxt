package check_test

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

func TestPrintfFormatChecking(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	// dot is a struct with one field of each basic kind.
	data := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "N", types.Typ[types.Int], false),
		types.NewField(0, testPkg.Types, "S", types.Typ[types.String], false),
		types.NewField(0, testPkg.Types, "B", types.Typ[types.Bool], false),
		types.NewField(0, testPkg.Types, "F", types.Typ[types.Float64], false),
	}, nil)

	checkTemplate := func(src string) error {
		tp, err := template.New[*types.Signature]("main").Parse(src)
		require.NoError(t, err)
		global := check.NewGlobal(testPkg.Types, testPkg.Fset, tp, check.DefaultFunctions(testPkg.Types))
		tree, ok := tp.FindTree("main")
		require.True(t, ok)
		return check.Execute(global, tree, data)
	}

	for _, tt := range []struct {
		name, template, wantErr string
	}{
		{"d with int", `{{printf "%d" .N}}`, ""},
		{"d with string", `{{printf "%d" .S}}`, "%d needs an integer but the argument is string"},
		{"d with float", `{{printf "%d" .F}}`, "%d needs an integer but the argument is float64"},
		{"d with bool", `{{printf "%d" .B}}`, "%d needs an integer but the argument is bool"},
		{"s with string", `{{printf "%s" .S}}`, ""},
		{"s with int", `{{printf "%s" .N}}`, "%s needs a string but the argument is int"},
		{"f with float", `{{printf "%.2f" .F}}`, ""},
		{"f with int", `{{printf "%f" .N}}`, "%f needs a floating-point number but the argument is int"},
		{"t with bool", `{{printf "%t" .B}}`, ""},
		{"t with int", `{{printf "%t" .N}}`, "%t needs a bool but the argument is int"},
		{"v with anything", `{{printf "%v %v %v %v" .N .S .B .F}}`, ""},
		{"multiple verbs ok", `{{printf "%d/%s=%t" .N .S .B}}`, ""},
		{"multiple verbs mismatch", `{{printf "%d %s" .S .N}}`, "%d needs an integer but the argument is string"},
		{"literal percent", `{{printf "100%% of %d" .N}}`, ""},
		{"star width", `{{printf "%*d" .N .N}}`, ""},
		{"star width wrong", `{{printf "%*d" .S .N}}`, "width (*) needs an integer"},
		{"too few arguments", `{{printf "%d %d" .N}}`, "not enough arguments"},
		{"too many arguments", `{{printf "%d" .N .S}}`, "too many arguments"},
		{"no verbs extra arg", `{{printf "hello" .N}}`, "too many arguments"},
		{"dynamic format not checked", `{{printf .S .N}}`, ""},
		{"piped argument", `{{.S | printf "%d"}}`, "%d needs an integer but the argument is string"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTemplate(tt.template)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
