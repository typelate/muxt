package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template/lex"
)

// TestParseTreeUndefinedFunction checks text/template's parse-time check
// that a called function is defined, which lex.Options.Functions turns on.
func TestParseTreeUndefinedFunction(t *testing.T) {
	for _, tt := range []struct {
		name      string
		text      string
		functions map[string]bool
		html      bool
		wantErr   string
	}{
		{name: "undefined", text: `{{foo .}}`, functions: map[string]bool{}, wantErr: `template: t:1: function "foo" not defined`},
		{name: "undefined in pipeline in attribute", text: `<p title="{{. | foo}}">x</p>`, functions: map[string]bool{}, html: true, wantErr: `template: t:1: function "foo" not defined`},
		{name: "undefined in define", text: `{{define "d"}}{{foo}}{{end}}`, functions: map[string]bool{}, wantErr: `template: t:1: function "foo" not defined`},
		{name: "undefined on a later line", text: "a\n{{if .}}\n{{foo}}{{end}}", functions: map[string]bool{}, wantErr: `template: t:3: function "foo" not defined`},
		{name: "html/template escaper is not defined", text: `{{_html_template_htmlescaper .}}`, functions: map[string]bool{}, html: true, wantErr: `template: t:1: function "_html_template_htmlescaper" not defined`},
		{name: "builtins", text: `{{and 1 2}}{{len .}}{{printf "%d" 1 | html}}{{urlquery .}}{{eq 1 1}}{{slice .}}`, functions: map[string]bool{}},
		{name: "registered", text: `{{foo .}}`, functions: map[string]bool{"foo": true}},
		{name: "no check", text: `{{foo .}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options := lex.NewOptions()
			options.ParseHTML = tt.html
			options.Functions = tt.functions
			_, err := ParseTree("t", "t", tt.text, nil, options)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
