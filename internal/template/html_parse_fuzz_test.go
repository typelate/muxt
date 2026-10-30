package template

import (
	"testing"
	"text/template/parse"

	"github.com/typelate/muxt/internal/template/lex"
)

// FuzzStdlibAcceptsImpliesHTMLParses checks that the HTML parser accepts
// every template text/template's parser accepts: muxt must parse each
// template html/template parses, whatever its markup.
func FuzzStdlibAcceptsImpliesHTMLParses(f *testing.F) {
	for _, seed := range []string{
		`<p>1 < 2</p>`,
		`I <3 templates`,
		`back <- here`,
		`a <`,
		`a</>b`,
		`</ >`,
		`<{{.Tag}}>tag</{{.Tag}}>`,
		`<h{{.N}}>heading</h{{.N}}>`,
		`<div data-{{.Name}}="x">attribute name</div>`,
		`<div {{/* c */}} class="a">comment in tag</div>`,
		`<div class="b" {{- /* why */ -}}>trimmed comment in tag</div>`,
		`<div class="{{/* c */}}a">x</div>`,
		`<div {{.Name}}="x">a</div>`,
		`<div ="x">a</div>`,
		`<p id=>x</p>`,
		`<a / b>x</a>`,
		`<p>a</p id>`,
		`<p>a</p {{.Name}}>`,
		`<a`,
		`<div class="x"`,
		`<div title="x`,
		`<p>a</p`,
		`a<!--z`,
		`a<!DOCTYPE`,
		`<script>{{/* c */}}a</script>`,
		`<textarea> {{- /* c */ -}} </textarea>`,
		`<script>a</script`,
		`<script>a</script x>b`,
		`{{if .X}}<script>a</script{{end}}`,
		`{{if .X}}<!{{end}}>`,
		`<p>a <! b {{.Y}}</p>`,
		`<p>a <? b {{.Y}}</p>`,
		`<p>a <!x {{.Y}}> b</p>`,
		`{{if .X}}<!-- {{end}} -->{{.Y}}`,
		`<!-- {{if .X}} -->x{{end}}{{.Y}}`,
		`{{if .X}}<!--{{else}}-->{{end}}{{.Y}}`,
		`<section><div data-{{.Name}}="x">a</div>b</section>`,
		`<div data-{{.Name}}="x"><div>a</div></div>`,
		`{{if .X}}<div class="x"{{end}}>`,
		`{{if .X}}<div class="x"{{else}}<div class="y"{{end}}>`,
		`{{define "open"}}<div class="x"{{end}}`,
		`<input {{if .X}}checked{{else}}disabled{{end}}>`,
		`<ul>{{range .Items}}<li>{{.}}{{end}}</ul>`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		stdlib := parse.New("t")
		stdlib.Mode = parse.ParseComments | parse.SkipFuncCheck
		if _, err := stdlib.Parse(text, "", "", make(map[string]*parse.Tree)); err != nil {
			t.Skip("text/template rejects it")
		}
		if _, err := ParseTree("t", "t", text, nil, lex.NewOptions()); err != nil {
			t.Fatalf("text/template parses %q, ParseTree with ParseHTML does not: %v", text, err)
		}
	})
}
