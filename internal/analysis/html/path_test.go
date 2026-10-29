package analysis

import (
	"slices"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// pathTo parses input and returns the path to the index-th (0-based)
// HTMLElementNode whose TagName matches tag, in pre-order.
func pathTo(t *testing.T, input, tag string, index int) (*template.Tree, []template.Node) {
	t.Helper()
	tree, err := template.ParseTree("test.gohtml", t.Name(), input, nil, lex.NewOptions())
	if err != nil {
		t.Fatalf("ParseTree(%q) error: %v", input, err)
	}
	var got []template.Node
	matches := 0
	template.WalkPath(tree, func(_ *template.Tree, path []template.Node, n template.Node) {
		if el, ok := n.(*template.HTMLElementNode); ok && el.TagName == tag {
			if matches == index && got == nil {
				got = append([]template.Node(nil), path...) // copy: path is reused
			}
			matches++
		}
	})
	if got == nil {
		t.Fatalf("no <%s> at index %d found in %q", tag, index, input)
	}
	return tree, got
}

func TestEnclosingElement(t *testing.T) {
	for _, test := range []struct {
		name, input, tag string
		wantTag          string // "" = want nil element
		wantMod          Modality
	}{
		{"direct child", `<table><div>x</div></table>`, "div", "table", ModalityAlways},
		{"through if", `<table>{{if .A}}<div>x</div>{{end}}</table>`, "div", "table", ModalityConditionally},
		{"through range", `<table>{{range .Rows}}<tr><td>x</td></tr>{{end}}</table>`, "tr", "table", ModalityRepeatedly},
		{"range then if keeps repeatedly", `<ul>{{range .A}}{{if .B}}<li>x</li>{{end}}{{end}}</ul>`, "li", "ul", ModalityRepeatedly},
		{"top level", `<div>x</div>`, "div", "", ModalityAlways},
		{"define is opaque", `{{define "row"}}<tr><td>x</td></tr>{{end}}<table></table>`, "tr", "", ModalityAlways},
		{"block is transparent", `<table>{{block "row" .}}<tr><td>x</td></tr>{{end}}</table>`, "tr", "table", ModalityAlways},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, path := pathTo(t, test.input, test.tag, 0)
			el, mod := enclosingElement(path)
			gotTag := ""
			if el != nil {
				gotTag = el.TagName
			}
			if gotTag != test.wantTag || mod != test.wantMod {
				t.Errorf("enclosingElement(path to <%s> in %q) = <%s>, %v; want <%s>, %v",
					test.tag, test.input, gotTag, mod, test.wantTag, test.wantMod)
			}
		})
	}
}

func TestElementAncestors(t *testing.T) {
	// select the second (index 1) <form> match: the inner one.
	_, path := pathTo(t, `<form>{{if .A}}<fieldset><form>x</form></fieldset>{{end}}</form>`, "form", 1)
	var tags []string
	var mods []Modality
	for el, mod := range elementAncestors(path) {
		tags = append(tags, el.TagName)
		mods = append(mods, mod)
	}
	want := []string{"fieldset", "form"}
	wantMods := []Modality{ModalityAlways, ModalityConditionally}
	if !slices.Equal(tags, want) {
		t.Errorf("elementAncestors tags = %v, want %v", tags, want)
	}
	if !slices.Equal(mods, wantMods) {
		t.Errorf("elementAncestors modalities = %v, want %v", mods, wantMods)
	}
}
