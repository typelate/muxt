package format

import (
	"slices"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// sourceSlices lists, in walk order, the source text the helpers in
// source.go give for each token the printer writes.
func sourceSlices(tree *template.Tree) []string {
	var got []string
	branch := func(b *template.BranchNode) {
		got = append(got, "open "+branchOpener(b), "else "+branchElse(b), "end "+branchEnd(b))
	}
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		switch n := n.(type) {
		case *template.IfNode:
			branch(&n.BranchNode)
		case *template.RangeNode:
			branch(&n.BranchNode)
		case *template.WithNode:
			branch(&n.BranchNode)
		case *template.DefineNode:
			got = append(got, "define "+defineOpener(n), "end "+defineEnd(n))
		case *template.BlockNode:
			got = append(got, "block "+blockOpener(n), "end "+blockEnd(n))
		case *template.HTMLElementNode:
			got = append(got, "tag "+startTagClose(n), "close "+closingTag(n))
		case *template.ActionNode, *template.TemplateNode, *template.BreakNode, *template.ContinueNode:
			got = append(got, "action "+actionSource(n))
		}
	})
	return got
}

func TestBranchSourceSlices(t *testing.T) {
	for _, tt := range []struct {
		name        string
		src         string
		left, right string
		want        []string
	}{
		{
			name: "else if chain",
			src:  `{{if .A}}a{{else if .B}}b{{else}}c{{end}}`,
			want: []string{
				"open {{if .A}}", "else {{else if .B}}", "end ",
				"open {{else if .B}}", "else {{else}}", "end {{end}}",
			},
		},
		{
			name: "long else if chain keeps its spacing",
			src:  `{{if .A}}a{{ else  if .B }}b{{- else if .C -}}c{{end }}`,
			want: []string{
				"open {{if .A}}", "else {{ else  if .B }}", "end ",
				"open {{ else  if .B }}", "else {{- else if .C -}}", "end ",
				"open {{- else if .C -}}", "else ", "end {{end }}",
			},
		},
		{
			name: "else holding a nested if is not a chain",
			src:  `{{if .A}}a{{else}}{{if .B}}b{{end}}{{end}}`,
			want: []string{
				"open {{if .A}}", "else {{else}}", "end {{end}}",
				"open {{if .B}}", "else ", "end {{end}}",
			},
		},
		{
			name: "else with chain",
			src:  `{{with .A}}a{{else with .B}}b{{end}}`,
			want: []string{
				"open {{with .A}}", "else {{else with .B}}", "end ",
				"open {{else with .B}}", "else ", "end {{end}}",
			},
		},
		{
			name: "action spacing",
			src:  `{{ if  .A }}x{{ end }}`,
			want: []string{"open {{ if  .A }}", "else ", "end {{ end }}"},
		},
		{
			name: "trim markers",
			src:  `{{- if .A -}}x{{- end -}}`,
			want: []string{"open {{- if .A -}}", "else ", "end {{- end -}}"},
		},
		{
			name: "empty bodies",
			src:  `{{if .A}}{{else}}{{end}}`,
			want: []string{"open {{if .A}}", "else {{else}}", "end {{end}}"},
		},
		{
			name: "range else",
			src:  `{{range $i, $v := .L}}x{{else}}y{{end}}`,
			want: []string{"open {{range $i, $v := .L}}", "else {{else}}", "end {{end}}"},
		},
		{
			name: "range break continue",
			src:  `{{range .L}}{{ break }}{{continue}}{{end}}`,
			want: []string{"open {{range .L}}", "else ", "end {{end}}", "action {{ break }}", "action {{continue}}"},
		},
		{
			name: "with",
			src:  `{{with .A}}x{{end}}`,
			want: []string{"open {{with .A}}", "else ", "end {{end}}"},
		},
		{
			name: "block",
			src:  `{{block "b" .}}x{{end}}`,
			want: []string{`block {{block "b" .}}`, "end {{end}}"},
		},
		{
			name: "block backtick name",
			src:  "{{block `a b` .}}x{{end}}",
			want: []string{"block {{block `a b` .}}", "end {{end}}"},
		},
		{
			name: "define trim markers",
			src:  `{{define "d" -}}x{{- end}}`,
			want: []string{`define {{define "d" -}}`, "end {{- end}}"},
		},
		{
			name: "define empty body",
			src:  "{{define `d`}}{{ end }}",
			want: []string{"define {{define `d`}}", "end {{ end }}"},
		},
		{
			name:  "custom delimiters",
			src:   `[[if .A]]x[[else]]y[[ end ]]`,
			left:  "[[",
			right: "]]",
			want:  []string{"open [[if .A]]", "else [[else]]", "end [[ end ]]"},
		},
		{
			name: "action and template call",
			src:  `{{ .X }}{{template "t" . }}`,
			want: []string{"action {{ .X }}", `action {{template "t" . }}`},
		},
		{
			name: "closing tag as written",
			src:  `<DIV>x</div >`,
			want: []string{"tag >", "close </div >"},
		},
		{
			name: "void slash",
			src:  `<p><br/><br /><img src="a" ></p>`,
			want: []string{
				"tag >", "close </p>",
				"tag />", "close ",
				"tag  />", "close ",
				"tag  >", "close ",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := template.ParseTree("t", "t", tt.src, nil, lex.Options{
				EmitComment: true,
				ParseHTML:   true,
				BreakOK:     true,
				ContinueOK:  true,
				LeftDelim:   tt.left,
				RightDelim:  tt.right,
			})
			if err != nil {
				t.Fatalf("ParseTree(%q) error: %v", tt.src, err)
			}
			if got := sourceSlices(tree); !slices.Equal(got, tt.want) {
				t.Errorf("source slices of %q =\n%q\nwant\n%q", tt.src, got, tt.want)
			}
		})
	}
}
