package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"
	muxthtml "github.com/typelate/htt/template"

	"github.com/typelate/muxt/internal/diagnostic"
)

func TestDiagnosticAt(t *testing.T) {
	const source = "<p>\n<!x></p>"
	set, err := muxthtml.New("page.gohtml").Parse(source)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	tree := set.Tree
	tree.Filepath = "/work/page.gohtml"

	for _, test := range []struct {
		name string
		in   htt.Diagnostic
		want diagnostic.Diagnostic
	}{
		{
			name: "a point",
			in: htt.Diagnostic{
				Pos: lex.Pos(len("<p>\n")), Severity: htt.SeverityError,
				Code: DiagEscapedMarkup, Message: "html/template escapes this",
			},
			want: diagnostic.Diagnostic{
				Severity: diagnostic.Error, Code: string(DiagEscapedMarkup), Message: "html/template escapes this",
				Place: diagnostic.Place{File: "/work/page.gohtml", Line: 2, Column: 1},
			},
		},
		{
			name: "a range",
			in: htt.Diagnostic{
				Pos: lex.Pos(len("<p>\n")), End: lex.Pos(len("<p>\n<!x>")), Severity: htt.SeverityWarning,
				Code: htt.DiagUnclosedElement, Message: "element <p> is not closed",
			},
			want: diagnostic.Diagnostic{
				Severity: diagnostic.Warning, Code: string(htt.DiagUnclosedElement), Message: "element <p> is not closed",
				Place: diagnostic.Place{File: "/work/page.gohtml", Line: 2, Column: 1, EndLine: 2, EndColumn: 5},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, diagnosticAt(tree, test.in))
		})
	}
}

func TestDiagnosticAt_parseName(t *testing.T) {
	set, err := muxthtml.New("page.gohtml").Parse("<!x>")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	got := diagnosticAt(set.Tree, htt.Diagnostic{Severity: htt.SeverityError}).Place.File
	if want := set.Tree.ParseName; got != want {
		t.Errorf("Place.File = %q, want the parse name %q when the tree has no path", got, want)
	}
}
