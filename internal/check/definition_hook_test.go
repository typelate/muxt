package check_test

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

// subDefinition is where {{define "sub"}}{{.}}{{end}} sits in a template
// file: the clause at the start, its name five bytes in, and the closing
// clause at offset 21.
var subDefinition = check.Definition{
	Name: "sub",
	Define: check.Span{
		Position: token.Position{Filename: "layout.gohtml", Offset: 0, Line: 1, Column: 1},
		Length:   16,
	},
	TemplateName: check.Span{
		Position: token.Position{Filename: "layout.gohtml", Offset: 9, Line: 1, Column: 10},
		Length:   5,
	},
	End: check.Span{
		Position: token.Position{Filename: "layout.gohtml", Offset: 21, Line: 1, Column: 22},
		Length:   7,
	},
}

func TestGlobalDefinitions(t *testing.T) {
	definitions := check.FindDefinitionFunc(func(name string) (check.Definition, bool) {
		if name != subDefinition.Name {
			return check.Definition{}, false
		}
		return subDefinition, true
	})

	inspect := func(t *testing.T, text string) []check.Definition {
		t.Helper()
		tmpl, err := template.New[*types.Signature]("main").Parse(text)
		require.NoError(t, err)
		pkg := types.NewPackage("example.com/app", "app")

		global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.DefaultFunctions(pkg))
		global.Definitions = definitions

		var got []check.Definition
		global.InspectTemplateNode = func(_ *template.TemplateNode, _ *template.Tree, _ types.Type, def check.Definition) {
			got = append(got, def)
		}
		tree, ok := tmpl.FindTree("main")
		require.True(t, ok)
		_ = check.Execute(global, tree, types.Typ[types.String])
		return got
	}

	t.Run("an invoked template reports where it was defined", func(t *testing.T) {
		got := inspect(t, `{{define "sub"}}{{.}}{{end}}{{template "sub" .}}`)

		require.Len(t, got, 1)
		assert.Equal(t, subDefinition, got[0])
	})

	t.Run("an undefined template reports a zero Definition", func(t *testing.T) {
		got := inspect(t, `{{template "missing" .}}`)

		require.Len(t, got, 1)
		assert.Equal(t, check.Definition{}, got[0])
		assert.False(t, got[0].Define.IsValid(), "a zero Definition has no valid position")
	})

	t.Run("a nil Definitions source reports a zero Definition", func(t *testing.T) {
		tmpl, err := template.New[*types.Signature]("main").Parse(`{{define "sub"}}{{.}}{{end}}{{template "sub" .}}`)
		require.NoError(t, err)
		pkg := types.NewPackage("example.com/app", "app")

		global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.DefaultFunctions(pkg))

		var got []check.Definition
		global.InspectTemplateNode = func(_ *template.TemplateNode, _ *template.Tree, _ types.Type, def check.Definition) {
			got = append(got, def)
		}
		tree, ok := tmpl.FindTree("main")
		require.True(t, ok)
		require.NoError(t, check.Execute(global, tree, types.Typ[types.String]))

		require.Len(t, got, 1)
		assert.Equal(t, check.Definition{}, got[0])
	})
}
