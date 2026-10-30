package check

import (
	"fmt"
	"go/token"
	"strings"
	"testing"
)

// formatDefSpan renders every coordinate a Span carries as
// filename:line:column@offset+length, so a test table pins all of them.
func formatDefSpan(s Span) string {
	if !s.IsValid() {
		return "-"
	}
	return fmt.Sprintf("%s:%d:%d@%d+%d", s.Filename, s.Line, s.Column, s.Offset, s.Length)
}

func formatDef(d Definition) string {
	return fmt.Sprintf("%q define=%s name=%s end=%s",
		d.Name,
		formatDefSpan(d.Define),
		formatDefSpan(d.TemplateName),
		formatDefSpan(d.End),
	)
}

func TestSpanString(t *testing.T) {
	for _, tt := range []struct {
		name string
		span Span
		want string
	}{
		{
			name: "a located span",
			span: Span{
				Position: token.Position{Filename: "a.gohtml", Line: 3, Column: 10, Offset: 11},
				Length:   3,
			},
			want: "a.gohtml:3:10+3",
		},
		{
			name: "a span that locates nothing",
			span: Span{},
			want: "-",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.span.String(); got != tt.want {
				t.Errorf("Span.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefinitionsIn(t *testing.T) {
	for _, tt := range []struct {
		name     string
		rootName string
		filename string
		text     string
		want     []string
	}{
		{
			name:     "a definition on the third line",
			rootName: "index.gohtml",
			filename: "index.gohtml",
			text:     "\n\n{{define `x`}}{{end}}",
			want: []string{
				"\"index.gohtml\" define=index.gohtml:1:1@0+0 name=- end=index.gohtml:3:22@23+0",
				"\"x\" define=index.gohtml:3:1@2+14 name=index.gohtml:3:10@11+3 end=index.gohtml:3:15@16+7",
			},
		},
		{
			name:     "columns are one based, unlike ErrorContext",
			rootName: "a.gohtml",
			filename: "a.gohtml",
			text:     `{{define "a"}}hi{{end}}`,
			want: []string{
				`"a.gohtml" define=a.gohtml:1:1@0+0 name=- end=a.gohtml:1:24@23+0`,
				`"a" define=a.gohtml:1:1@0+14 name=a.gohtml:1:10@9+3 end=a.gohtml:1:17@16+7`,
			},
		},
		{
			name:     "an end clause on a later line",
			rootName: "a.gohtml",
			filename: "a.gohtml",
			text:     "{{define \"a\"}}\nhi\n{{end}}\n",
			want: []string{
				`"a.gohtml" define=a.gohtml:1:1@0+0 name=- end=a.gohtml:3:9@26+0`,
				`"a" define=a.gohtml:1:1@0+14 name=a.gohtml:1:10@9+3 end=a.gohtml:3:1@18+7`,
			},
		},
		{
			name:     "carriage returns count toward the column",
			rootName: "a.gohtml",
			filename: "a.gohtml",
			text:     "a\r\n{{define \"b\"}}x{{end}}",
			want: []string{
				`"a.gohtml" define=a.gohtml:1:1@0+0 name=- end=a.gohtml:2:23@25+0`,
				`"b" define=a.gohtml:2:1@3+14 name=a.gohtml:2:10@12+3 end=a.gohtml:2:16@18+7`,
			},
		},
		{
			name:     "sibling definitions across lines",
			rootName: "a.gohtml",
			filename: "a.gohtml",
			text:     "{{define \"a\"}}A{{end}}\n{{define \"b\"}}B{{end}}",
			want: []string{
				`"a.gohtml" define=a.gohtml:1:1@0+0 name=- end=a.gohtml:2:23@45+0`,
				`"a" define=a.gohtml:1:1@0+14 name=a.gohtml:1:10@9+3 end=a.gohtml:1:16@15+7`,
				`"b" define=a.gohtml:2:1@23+14 name=a.gohtml:2:10@32+3 end=a.gohtml:2:16@38+7`,
			},
		},
		{
			name:     "a definition with no matching end has an invalid end span",
			rootName: "a.gohtml",
			filename: "a.gohtml",
			text:     `{{define "a"}}x`,
			want: []string{
				`"a.gohtml" define=a.gohtml:1:1@0+0 name=- end=a.gohtml:1:16@15+0`,
				`"a" define=a.gohtml:1:1@0+14 name=a.gohtml:1:10@9+3 end=-`,
			},
		},
		{
			name:     "a file with no definitions yields only its root",
			rootName: "a.gohtml",
			filename: "a.gohtml",
			text:     `hello {{.X}}`,
			want: []string{
				`"a.gohtml" define=a.gohtml:1:1@0+0 name=- end=a.gohtml:1:13@12+0`,
			},
		},
		{
			name:     "an empty file",
			rootName: "e.gohtml",
			filename: "e.gohtml",
			text:     "",
			want: []string{
				`"e.gohtml" define=e.gohtml:1:1@0+0 name=- end=e.gohtml:1:1@0+0`,
			},
		},
		{
			// go/token does not begin a line for a trailing newline, so
			// neither does this: a file ending in "\n" ends on its last
			// line, one column past the newline.
			name:     "a trailing newline does not start a line",
			rootName: "a.gohtml",
			filename: "a.gohtml",
			text:     "hi\n",
			want: []string{
				`"a.gohtml" define=a.gohtml:1:1@0+0 name=- end=a.gohtml:1:4@3+0`,
			},
		},
		{
			name:     "a root template named separately from its file",
			rootName: "app",
			filename: "main.go",
			text:     `{{define "a"}}x{{end}}`,
			want: []string{
				`"app" define=main.go:1:1@0+0 name=- end=main.go:1:23@22+0`,
				`"a" define=main.go:1:1@0+14 name=main.go:1:10@9+3 end=main.go:1:16@15+7`,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := definitionsIn(tt.rootName, tt.filename, tt.text, "", "")

			var formatted []string
			for _, d := range got {
				formatted = append(formatted, formatDef(d))
			}
			if strings.Join(formatted, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("definitionsIn(%q, %q, %q) =\n\t%s\nwant\n\t%s",
					tt.rootName, tt.filename, tt.text,
					strings.Join(formatted, "\n\t"), strings.Join(tt.want, "\n\t"))
			}
		})
	}
}
