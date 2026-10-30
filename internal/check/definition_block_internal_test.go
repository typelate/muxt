package check

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template/lex"

	"github.com/typelate/muxt/internal/template"
)

// pipelineText renders a pipeline's commands as written. muxt's parser
// ends a PipeNode at the action's right delimiter, so PipeNode.String
// includes the "}}" that upstream's text/template/parse leaves out.
func pipelineText(p *template.PipeNode) string {
	cmds := make([]string, 0, len(p.Commands))
	for _, c := range p.Commands {
		cmds = append(cmds, c.String())
	}
	return strings.Join(cmds, " | ")
}

func TestDefinitionBlockPipeline(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
		// blocks maps a template name to the pipeline its block clause
		// passes. A name absent from the map was defined by a define
		// clause, or is the file's own root template, and carries no
		// pipeline.
		blocks map[string]string
	}{
		{
			name:   "a define clause is not a block",
			text:   `{{define "d"}}x{{end}}`,
			blocks: nil,
		},
		{
			name:   "a block carries the pipeline it was written with",
			text:   `{{block "b" .}}x{{end}}`,
			blocks: map[string]string{"b": "."},
		},
		{
			name:   "a block passed a field",
			text:   `{{block "b" .Field}}x{{end}}`,
			blocks: map[string]string{"b": ".Field"},
		},
		{
			name:   "a define and a block side by side",
			text:   `{{define "d"}}x{{end}}{{block "b" .Y}}y{{end}}`,
			blocks: map[string]string{"b": ".Y"},
		},
		{
			// parse puts the inner block's invocation in the outer
			// block's own tree, not in the root.
			name:   "a block nested in a block",
			text:   `{{block "outer" .}}{{block "inner" .X}}i{{end}}{{end}}`,
			blocks: map[string]string{"outer": ".", "inner": ".X"},
		},
		{
			// And a block inside a branch nests below that branch's list.
			name:   "a block nested in an if",
			text:   `{{block "o" .}}{{if .C}}{{block "i" .Y}}x{{end}}{{end}}{{end}}`,
			blocks: map[string]string{"o": ".", "i": ".Y"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, def := range definitionsIn("index.gohtml", "index.gohtml", tt.text, lex.NewOptions()) {
				want, isBlock := tt.blocks[def.Name]

				if def.IsBlock() != isBlock {
					t.Errorf("Definition(%q).IsBlock() = %t, want %t", def.Name, def.IsBlock(), isBlock)
					continue
				}
				if !isBlock {
					if got := def.BlockPipeline(); got != nil {
						t.Errorf("Definition(%q).BlockPipeline() = %s, want nil", def.Name, got)
					}
					continue
				}
				got := def.BlockPipeline()
				if got == nil {
					t.Fatalf("Definition(%q).BlockPipeline() = nil, want %q", def.Name, want)
				}
				if pipelineText(got) != want {
					t.Errorf("Definition(%q).BlockPipeline() = %q, want %q", def.Name, pipelineText(got), want)
				}
			}
		})
	}
}
