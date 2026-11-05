package format

import (
	"errors"
	"testing"

	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/lex"
)

// withTestEdits seeds the emitter with edits, as a formatter bug might.
func withTestEdits(edits ...edit) Option {
	return func(f *formatter) { f.testEdits = edits }
}

// TestOverlappingEdits checks that two edits covering the same bytes make
// Source return errOverlappingEdits, writing neither, and make Node fall
// back to the source as written.
func TestOverlappingEdits(t *testing.T) {
	const src = "<div>\n    <p>a</p>\n</div>\n"
	overlap := withTestEdits(edit{start: 5, end: 10, text: "\n\t"}, edit{start: 7, end: 9, text: ""})

	out, err := Source("overlap.gohtml", src, overlap)
	if !errors.Is(err, errOverlappingEdits) || out != "" {
		t.Errorf("Source(%q) with overlapping edits = %q, %v; want \"\", errOverlappingEdits", src, out, err)
	}

	tree, err := template.ParseTree("overlap.gohtml", "overlap.gohtml", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Node(tree.Root, overlap), tree.Root.String(); got != want {
		t.Errorf("Node(%q) with overlapping edits = %q, want the source as written %q", src, got, want)
	}

	// Without them, both reindent.
	if out, err := Source("overlap.gohtml", src); err != nil || out != "<div>\n\t<p>a</p>\n</div>\n" {
		t.Errorf("Source(%q) = %q, %v; want the reindented source", src, out, err)
	}
}
