package mutation

import (
	"go/types"
	"testing"

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
		trees, err := asteval.ParseTrees("t", tt.text, "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := complexity(trees["t"].Root); got != tt.want {
			t.Errorf("complexity(%s) = %d, want %d", tt.text, got, tt.want)
		}
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
		if got := typeKey(tt.typ); got != tt.key {
			t.Errorf("typeKey(%v) = %q, want %q", tt.typ, got, tt.key)
		}
		if got := typeDisplay(tt.typ); got != tt.display {
			t.Errorf("typeDisplay(%v) = %q, want %q", tt.typ, got, tt.display)
		}
	}
}
