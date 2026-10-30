package check

import (
	"testing"
)

func TestDefineClauses(t *testing.T) {
	// Want describes one clause the scanner should find. Each span is
	// given as the text it must cover and the offset it must start at,
	// so a row says what was matched rather than only where.
	//
	// An offset of -1 means the span locates nothing, which is what a
	// definition left open by malformed input has for its end.
	type Want struct {
		Name string

		Define   string
		DefineAt int

		NameLiteral   string
		NameLiteralAt int

		End   string
		EndAt int
	}

	// span reports the text a span covers and where it begins.
	span := func(text string, s textSpan) (string, int) {
		if !s.IsValid() {
			return "", -1
		}
		return text[s.Off : s.Off+s.Len], s.Off
	}

	for _, tt := range []struct {
		name        string
		text        string
		left, right string
		want        []Want
	}{
		{
			name: "a single define",
			text: `{{define "a"}}hi{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 16,
			}},
		},
		{
			name: "a block defines a template too",
			text: `{{block "b" .}}hi{{end}}`,
			want: []Want{{
				Name:        "b",
				Define:      `{{block "b" .}}`,
				NameLiteral: `"b"`, NameLiteralAt: 8,
				End: `{{end}}`, EndAt: 17,
			}},
		},
		{
			name:  "custom delimiters",
			text:  `[[define "a"]]hi[[end]]`,
			left:  "[[",
			right: "]]",
			want: []Want{{
				Name:        "a",
				Define:      `[[define "a"]]`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `[[end]]`, EndAt: 16,
			}},
		},
		{
			name: "trim markers belong to the clause they decorate",
			text: "{{define \"a\" -}}\n  hi\n{{- end}}",
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a" -}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{- end}}`, EndAt: 22,
			}},
		},
		{
			name: "an empty body still has a matching end",
			text: `{{define "a"}}{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 14,
			}},
		},
		{
			name: "a nested if does not steal the end",
			text: `{{define "a"}}{{if .X}}y{{end}}{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 31,
			}},
		},
		{
			name: "else does not change nesting depth",
			text: `{{define "a"}}{{if .X}}y{{else}}z{{end}}{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 40,
			}},
		},
		{
			name: "nested range and with each consume an end",
			text: `{{define "a"}}{{range .X}}{{with .Y}}z{{end}}{{end}}{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 52,
			}},
		},
		{
			// The template lexer ends an identifier at the first byte
			// that cannot continue one, so these open a block just as
			// {{if .X}} does and must consume their own end clause.
			name: "a block keyword followed immediately by a field",
			text: `{{define "a"}}{{if.X}}y{{end}}{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 30,
			}},
		},
		{
			name: "a range keyword followed immediately by a field",
			text: `{{define "a"}}{{range.X}}y{{end}}{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 33,
			}},
		},
		{
			name: "a block keyword followed immediately by a parenthesis",
			text: `{{define "a"}}{{if(eq 1 1)}}y{{end}}{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 36,
			}},
		},
		{
			name: "a right delimiter inside a quoted argument is not a delimiter",
			text: `{{define "a"}}{{if eq .X "}}"}}y{{end}}{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 39,
			}},
		},
		{
			name: "a left delimiter inside a raw quoted argument is not a delimiter",
			text: "{{define \"a\"}}{{if eq .X `{{`}}y{{end}}{{end}}",
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 39,
			}},
		},
		{
			name: "a define inside a comment is not a definition",
			text: `{{/* {{define "fake"}} */}}{{define "real"}}x{{end}}`,
			want: []Want{{
				Name:   "real",
				Define: `{{define "real"}}`, DefineAt: 27,
				NameLiteral: `"real"`, NameLiteralAt: 36,
				End: `{{end}}`, EndAt: 45,
			}},
		},
		{
			// A comment may carry a trim marker. Unless it is recognised
			// as a comment the scan stops at the first right delimiter in
			// the body, and the comment's own end clause is then mistaken
			// for the define's.
			name: "a left trimmed comment hiding a delimiter and an end clause",
			text: `{{define "a"}}{{- /* a}}b {{end}} c */ -}}body{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 46,
			}},
		},
		{
			// Same failure, seen from the other side: a define clause
			// inside a trimmed comment must not become a definition.
			name: "a left trimmed comment hiding a define clause",
			text: `{{define "a"}}{{- /* x}} {{define "fake"}}y{{end}} */ -}}b{{end}}`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 58,
			}},
		},
		{
			name: "a name containing the right delimiter",
			text: `{{define "a}}b"}}x{{end}}`,
			want: []Want{{
				Name:        `a}}b`,
				Define:      `{{define "a}}b"}}`,
				NameLiteral: `"a}}b"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 18,
			}},
		},
		{
			name: "a name containing an escaped quote",
			text: `{{define "a\"b"}}x{{end}}`,
			want: []Want{{
				Name:        `a"b`,
				Define:      `{{define "a\"b"}}`,
				NameLiteral: `"a\"b"`, NameLiteralAt: 9,
				End: `{{end}}`, EndAt: 18,
			}},
		},
		{
			name: "a raw quoted name",
			text: "{{define `a`}}x{{end}}",
			want: []Want{{
				Name:        "a",
				Define:      "{{define `a`}}",
				NameLiteral: "`a`", NameLiteralAt: 9,
				End: "{{end}}", EndAt: 15,
			}},
		},
		{
			name: "sibling definitions are returned in source order",
			text: `{{define "a"}}A{{end}}{{define "b"}}B{{end}}`,
			want: []Want{
				{
					Name:        "a",
					Define:      `{{define "a"}}`,
					NameLiteral: `"a"`, NameLiteralAt: 9,
					End: `{{end}}`, EndAt: 15,
				},
				{
					Name:   "b",
					Define: `{{define "b"}}`, DefineAt: 22,
					NameLiteral: `"b"`, NameLiteralAt: 31,
					End: `{{end}}`, EndAt: 37,
				},
			},
		},
		{
			name: "a definition on the third line",
			text: "\n\n{{define `x`}}{{end}}",
			want: []Want{{
				Name:   "x",
				Define: "{{define `x`}}", DefineAt: 2,
				NameLiteral: "`x`", NameLiteralAt: 11,
				End: "{{end}}", EndAt: 16,
			}},
		},
		{
			name: "text without definitions",
			text: `hello {{.X}} world`,
			want: nil,
		},
		{
			name: "a define with no matching end has an invalid end span",
			text: `{{define "a"}}x`,
			want: []Want{{
				Name:        "a",
				Define:      `{{define "a"}}`,
				NameLiteral: `"a"`, NameLiteralAt: 9,
				EndAt: -1,
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clauses := defineClauses(tt.text, tt.left, tt.right)

			got := make([]Want, 0, len(clauses))
			for _, c := range clauses {
				var w Want
				w.Name = c.Name
				w.Define, w.DefineAt = span(tt.text, c.Define)
				w.NameLiteral, w.NameLiteralAt = span(tt.text, c.NameLiteral)
				w.End, w.EndAt = span(tt.text, c.End)
				got = append(got, w)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("defineClauses(%q) found %d definitions, want %d:\n\tgot  %+v\n\twant %+v",
					tt.text, len(got), len(tt.want), got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("defineClauses(%q) definition %d =\n\t%+v\nwant\n\t%+v",
						tt.text, i, got[i], tt.want[i])
				}
			}
		})
	}
}
