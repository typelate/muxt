package format

import (
	"errors"
	"testing"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// TestEmitOverlappingEdits checks that the emitter reports two edits that
// cover the same bytes as errOverlappingEdits instead of writing either.
func TestEmitOverlappingEdits(t *testing.T) {
	const src = "<div>\n    <p>a</p>\n</div>\n"
	tree, err := template.ParseTree("overlap.gohtml", "overlap.gohtml", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	styles := analysis.ComputeWhitespace([]*template.Tree{tree}, analysis.WhitespaceOptions{})
	e := newEmitter(src, 0, classify(tree.Root, styles), nil, nil)
	e.edits = []edit{{start: 5, end: 10, text: "\n\t"}, {start: 7, end: 9, text: ""}}
	out, err := e.emit(tree.Root)
	if !errors.Is(err, errOverlappingEdits) {
		t.Errorf("emit(%q) with overlapping edits = %q, %v; want errOverlappingEdits", src, out, err)
	}
}
