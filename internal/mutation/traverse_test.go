package mutation

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/asteval"
)

// TestComplexity states how a template's cyclomatic complexity is counted:
// one, plus one per branch point, where each and or or in a branch's
// pipeline is a branch point of its own.
func TestComplexity(t *testing.T) {
	for _, tt := range []struct {
		text string
		want int
	}{
		{text: `static`, want: 1},
		{text: `{{.A}}{{template "x" .}}`, want: 1},
		{text: `{{if .A}}{{end}}`, want: 2},
		{text: `{{if and .A .B}}{{end}}`, want: 3},
		{text: `{{with or .A .B}}{{end}}`, want: 3},
		{text: `{{range .A}}{{if .B}}{{end}}{{else}}{{end}}`, want: 3},
		{text: `{{if .A}}{{else if .B}}{{end}}`, want: 3},
		{text: `{{if .A}}{{else}}{{with .B}}{{end}}{{end}}`, want: 3},
	} {
		t.Run(tt.text, func(t *testing.T) {
			trees, err := asteval.ParseTrees("t", tt.text, "", "", nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, complexity(trees["t"].Root), "complexity(%s)", tt.text)
		})
	}
}

// TestTypeKeyUnaliases states that two spellings of one type share a key,
// so an alias hides neither a change to what it stands for nor sameness.
func TestTypeKeyUnaliases(t *testing.T) {
	pkg := types.NewPackage("example.com/data", "data")
	data := types.NewNamed(types.NewTypeName(0, pkg, "Data", nil), types.NewStruct(nil, nil), nil)
	alias := types.NewAlias(types.NewTypeName(0, pkg, "Page", nil), data)
	for _, tt := range []struct {
		name         string
		alias, plain types.Type
	}{
		{name: "alias", alias: alias, plain: data},
		{name: "pointer to alias", alias: types.NewPointer(alias), plain: types.NewPointer(data)},
		{name: "map of slices of alias", alias: types.NewMap(types.Typ[types.String], types.NewSlice(alias)), plain: types.NewMap(types.Typ[types.String], types.NewSlice(data))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, typeKey(tt.plain), typeKey(tt.alias), "typeKey(%v)", tt.alias)
		})
	}
}

// TestTypeNames states the two ways a type is written: exactly, with its
// full package path, to decide whether a template was already mutated with
// it, and by package name for a reader.
func TestTypeNames(t *testing.T) {
	page := dataType(t, pageSource, "Page")
	for _, tt := range []struct {
		typ          types.Type
		key, display string
	}{
		{typ: nil, key: "<nil>", display: "<nil>"},
		{typ: types.Typ[types.Int], key: "int", display: "int"},
		{typ: page, key: "example.com/data.Page", display: "data.Page"},
		{typ: types.NewPointer(page), key: "*example.com/data.Page", display: "*data.Page"},
	} {
		t.Run(tt.key, func(t *testing.T) {
			assert.Equal(t, tt.key, typeKey(tt.typ), "typeKey(%v)", tt.typ)
			assert.Equal(t, tt.display, typeDisplay(tt.typ), "typeDisplay(%v)", tt.typ)
		})
	}
}
