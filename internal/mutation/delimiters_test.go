package mutation

import (
	"go/token"
	"strings"
	"testing"

	"github.com/typelate/check"
)

// TestDelimitersReadsThemOffTheEndClause states how the delimiters a
// template set was built with are recovered.
//
// Nothing exposes them: text/template keeps its delimiters to itself, and
// check evaluates Delims without reporting the result. What check does
// give is a span running from the left delimiter through the right one,
// and an end clause holds nothing else but the word and its trim markers.
//
// Getting this wrong is silent. Reading a template with the wrong
// delimiters produces a tree with no actions in it, and a run over that
// finds nothing to mutate.
func TestDelimitersReadsThemOffTheEndClause(t *testing.T) {
	for _, tt := range []struct {
		name  string
		end   string
		left  string
		right string
		ok    bool
	}{
		{name: "the defaults", end: "{{end}}", left: "{{", right: "}}", ok: true},
		{name: "square brackets", end: "[[end]]", left: "[[", right: "]]", ok: true},
		{name: "delimiters of different lengths", end: "<%end%>", left: "<%", right: "%>", ok: true},
		{
			name: "trim markers are not part of a delimiter",
			end:  "[[- end -]]", left: "[[", right: "]]", ok: true,
		},
		{
			name: "a marker without a space is still not part of one",
			end:  "{{-end-}}", left: "{{", right: "}}", ok: true,
		},
		{
			name: "a delimiter that ends in a dash keeps it",
			// The marker is what follows the delimiter, and it is
			// stripped once. A delimiter written with its own dash is
			// still the delimiter.
			end: "{-end-}", left: "{", right: "}", ok: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// The clause is placed after some text so the span is not
			// trivially at offset zero.
			const prefix = "hello "
			text := prefix + tt.end
			definition := check.Definition{
				Name:         "x",
				Define:       check.Span{Position: token.Position{Filename: "t.gohtml"}, Length: 1},
				TemplateName: check.Span{Position: token.Position{Filename: "t.gohtml", Offset: 1, Line: 1}, Length: 3},
				End: check.Span{
					Position: token.Position{Filename: "t.gohtml", Offset: len(prefix)},
					Length:   len(tt.end),
				},
			}

			left, right, ok := delimiters(text, definition)
			if ok != tt.ok {
				t.Fatalf("delimiters(%q) ok = %t, want %t", tt.end, ok, tt.ok)
			}
			if left != tt.left || right != tt.right {
				t.Errorf("delimiters(%q) = %q, %q, want %q, %q", tt.end, left, right, tt.left, tt.right)
			}
		})
	}
}

// TestDelimitersDeclinesWhatItCannotRead states that a definition with no
// end clause to read is declined rather than guessed at.
//
// The caller falls back to the defaults, which is right for a template
// that has no define clause: it is the whole file, and the file's other
// definitions answer for it.
func TestDelimitersDeclinesWhatItCannotRead(t *testing.T) {
	const text = `{{define "x"}}{{end}}`

	t.Run("a template with no define clause", func(t *testing.T) {
		definition := check.Definition{
			Name: "t.gohtml",
			End:  check.Span{Position: token.Position{Filename: "t.gohtml", Offset: len(text)}},
		}
		if _, _, ok := delimiters(text, definition); ok {
			t.Error("delimiters accepted a definition with no end clause to read")
		}
	})

	t.Run("a span outside the text", func(t *testing.T) {
		definition := check.Definition{
			Name:         "x",
			TemplateName: check.Span{Position: token.Position{Filename: "t.gohtml", Offset: 9, Line: 1}, Length: 3},
			End: check.Span{
				Position: token.Position{Filename: "t.gohtml", Offset: len(text)},
				Length:   99,
			},
		}
		if _, _, ok := delimiters(text, definition); ok {
			t.Error("delimiters read past the end of the text")
		}
	})
}

// TestDelimitersAgreeWithTheScanner states that the delimiters read off a
// definition are ones the action scanner can then use.
//
// The two are separate pieces of arithmetic over the same text, and a
// pair that reads out cleanly but finds no actions would leave a template
// with nothing to mutate and no error.
func TestDelimitersAgreeWithTheScanner(t *testing.T) {
	const text = `[[define "greeting"]]Hello, [[.Name]]![[end]]`

	endAt := strings.LastIndex(text, "[[end]]")
	definition := check.Definition{
		Name:         "greeting",
		Define:       check.Span{Position: token.Position{Filename: "t.gohtml"}, Length: 21},
		TemplateName: check.Span{Position: token.Position{Filename: "t.gohtml", Offset: 9, Line: 1}, Length: 10},
		End:          check.Span{Position: token.Position{Filename: "t.gohtml", Offset: endAt}, Length: len("[[end]]")},
	}

	left, right, ok := delimiters(text, definition)
	if !ok {
		t.Fatal("delimiters could not read the pair off the end clause")
	}

	found := regions(text, left, right)
	if len(found) != 3 {
		t.Fatalf("regions = %d, want the define, the action and the end", len(found))
	}
	if got := text[found[1].start:found[1].end]; got != "[[.Name]]" {
		t.Errorf("second region = %q, want %q", got, "[[.Name]]")
	}

	// The defaults find nothing here, which is the failure this whole
	// derivation exists to avoid.
	if none := regions(text, "", ""); len(none) != 0 {
		t.Errorf("scanning with the default delimiters found %d regions, want none", len(none))
	}
}
