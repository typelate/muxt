package analysis

import (
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/template"
	muxthtml "github.com/typelate/muxt/template/html"
)

// stdlibOutcome is what the standard library's html/template does with a
// test input: refuse to execute it, or write output that has and lacks
// some text. A row's diagnostics are claims about html/template, and the
// test executes the input to check them.
type stdlibOutcome struct {
	refuses    bool
	has, lacks []string
}

func TestCheckComments(t *testing.T) {
	const (
		bang      = `html/template escapes this "<!", so browsers show it as text; comment with {{/* */}} or <!-- -->`
		pi        = `html/template escapes this "<?", so browsers show it as text; comment with {{/* */}} or <!-- -->`
		title     = `html/template writes this <!-- as text inside <title>; browsers show it`
		area      = `html/template writes this <!-- as text inside <textarea>; browsers show it`
		refuses   = `html/template refuses this template: `
		rawBang   = `html/template escapes this "<!" as "&lt;", but browsers read <%s> content as raw text and show the "&lt;"`
		rawPI     = `html/template escapes this "<?" as "&lt;", but browsers read <%s> content as raw text and show the "&lt;"`
		rawStrips = `html/template strips this comment, but browsers read <%s> content as raw text and keep it`
	)
	for _, test := range []struct {
		name, input string
		want        []string
		stdlib      stdlibOutcome
	}{
		{name: "comment in text", input: `<p>a<!-- c -->b</p>`, stdlib: stdlibOutcome{has: []string{"<p>ab</p>"}}},
		{name: "action in comment", input: `<p><!-- {{.X}} --></p>`, stdlib: stdlibOutcome{has: []string{"<p></p>"}}},
		{name: "control in comment", input: `<p><!-- a {{if .X}}b{{end}} --></p>`, stdlib: stdlibOutcome{has: []string{"<p></p>"}}},
		{name: "doctype", input: "<!DOCTYPE html>\n<html></html>", stdlib: stdlibOutcome{has: []string{"<!DOCTYPE html>"}}},
		{name: "lower case doctype", input: `<!doctype html>`, stdlib: stdlibOutcome{has: []string{"<!doctype html>"}}},
		{name: "comment in attribute", input: `<p title="<!-- c -->">x</p>`, stdlib: stdlibOutcome{has: []string{`title="<!-- c -->"`}}},
		{name: "bang in attribute", input: `<p title="<!x>">x</p>`, stdlib: stdlibOutcome{has: []string{`title="<!x>"`}}},
		{name: "comment in script", input: `<script>/*<!-- c -->*/</script>`, stdlib: stdlibOutcome{lacks: []string{"<!--"}}},
		{name: "bang in script", input: `<script>if (a <!x) {}</script>`, stdlib: stdlibOutcome{has: []string{"<script>if (a"}}},
		{name: "comments and less than in script", input: "<script>/* a */ if (a < b) {} // c\nvar s = '</script' + '>'</script>", stdlib: stdlibOutcome{has: []string{"a < b", `\x3C/script`}, lacks: []string{"/* a */", "// c"}}},
		{name: "comment in style", input: `<style><!-- c --></style>`, stdlib: stdlibOutcome{has: []string{"<style>"}}},
		{name: "css comment", input: `<style>p { /* c */ color: red }</style>`, stdlib: stdlibOutcome{lacks: []string{"/* c */"}}},
		{name: "text in title", input: `<title>a < b</title>`, stdlib: stdlibOutcome{has: []string{"a &lt; b"}}},
		{name: "empty comment", input: `<p><!----></p>`, stdlib: stdlibOutcome{has: []string{"<p></p>"}}},

		{name: "comment in title", input: `<title>T<!-- c --></title>`, want: []string{"1:9: " + title}, stdlib: stdlibOutcome{has: []string{"T&lt;!-- c -->"}}},
		{name: "comment in textarea", input: `<TEXTAREA>T<!-- c --></TEXTAREA>`, want: []string{"1:12: " + area}, stdlib: stdlibOutcome{has: []string{"T&lt;!-- c -->"}}},
		{name: "comment after action in title", input: "<title>{{.T}}\n<!-- c --></title>", want: []string{"2:1: " + title}, stdlib: stdlibOutcome{has: []string{"&lt;!-- c -->"}}},
		{name: "comment in svg title", input: `<svg><title><!-- c --></title></svg>`, want: []string{"1:13: " + title}, stdlib: stdlibOutcome{has: []string{"&lt;!-- c -->"}}},
		{name: "bang in title", input: `<title><!x></title>`, stdlib: stdlibOutcome{has: []string{"&lt;!x>"}}},

		{name: "bogus comment", input: `<p><!x></p>`, want: []string{"1:4: " + bang}, stdlib: stdlibOutcome{has: []string{"&lt;!x>"}}},
		{name: "processing instruction", input: `<p><?x></p>`, want: []string{"1:4: " + pi}, stdlib: stdlibOutcome{has: []string{"&lt;?x>"}}},
		{name: "conditional comment", input: "<div>\n  <![if !IE]>a<![endif]>\n</div>", want: []string{"2:3: " + bang, "2:15: " + bang}, stdlib: stdlibOutcome{has: []string{"&lt;![if !IE]>a&lt;![endif]>"}}},
		{name: "bang before action", input: `<p><!{{.X}}></p>`, want: []string{"1:4: " + bang}, stdlib: stdlibOutcome{has: []string{"&lt;!true>"}}},
		{name: "bang in branch", input: `<p>{{if .X}}<!{{end}}x></p>`, want: []string{"1:13: " + bang}, stdlib: stdlibOutcome{has: []string{"&lt;!x>"}}},
		{name: "less than", input: `<p>1 < 2 <3</p>`, stdlib: stdlibOutcome{has: []string{"1 &lt; 2 &lt;3"}}},

		{name: "bang in quoted value of unread tag", input: `<li{{if .X}} title="<!x>"{{end}}>a</li>`, stdlib: stdlibOutcome{has: []string{`title="<!x>"`}}},
		{name: "question in single-quoted value of unread tag", input: `<li{{if .X}} title='a <?x>'{{end}}>a</li>`, stdlib: stdlibOutcome{has: []string{`title='a <?x>'`}}},
		{name: "bang in branch content", input: `<p>{{if .X}}<!x>{{end}}</p>`, want: []string{"1:13: " + bang}, stdlib: stdlibOutcome{has: []string{"&lt;!x>"}}},
		// Review finding 1: html/template does not show this <!x>; it
		// refuses to render the template.
		{name: "bang after closed value of unread tag", input: `<li{{if .X}} title="a" <!x>{{end}}>a</li>`, want: []string{"1:1: " + refuses}, stdlib: stdlibOutcome{refuses: true}},
		// Review finding 2: a > in a quoted value does not end the tag.
		{name: "bang after greater than in quoted value of unread tag", input: `<li{{if .X}} title="a>b <!x>"{{end}}>a</li>`, stdlib: stdlibOutcome{has: []string{`title="a>b <!x>"`}}},

		{name: "comments in title branches", input: `<title>{{if .A}}<!-- a -->{{else}}<!-- b -->{{end}}{{range .R}}<!-- c -->{{end}}{{with .W}}<!-- d -->{{end}}</title>`, want: []string{
			"1:17: " + title, "1:35: " + title, "1:64: " + title, "1:92: " + title,
		}, stdlib: stdlibOutcome{has: []string{"&lt;!-- a -->&lt;!-- c -->&lt;!-- d -->"}}},
		{name: "range and with content", input: `<p>{{range .R}}<!x>{{else}}<?y>{{end}}{{with .W}}<!-->a-->{{end}}</p>`, want: []string{
			"1:16: " + bang,
			"1:28: " + pi,
			`1:50: browsers end this comment at the "<!-->" at 1:50 but html/template strips through the "-->" at 1:56, hiding the markup between`,
		}, stdlib: stdlibOutcome{has: []string{"<p>&lt;!x></p>"}}},

		{name: "abrupt comment", input: "<p><!-->a\n<b>x</b> --></p>", want: []string{
			`1:4: browsers end this comment at the "<!-->" at 1:4 but html/template strips through the "-->" at 2:10, hiding the markup between`,
		}, stdlib: stdlibOutcome{has: []string{"<p></p>"}}},
		{name: "abrupt comment with dash", input: `<p><!--->a <b>x</b>--></p>`, want: []string{
			`1:4: browsers end this comment at the "<!--->" at 1:4 but html/template strips through the "-->" at 1:20, hiding the markup between`,
		}, stdlib: stdlibOutcome{has: []string{"<p></p>"}}},
		{name: "abrupt comment never closed", input: `<p><!-->a</p>`, want: []string{"1:1: " + refuses + "a comment that never closes"}, stdlib: stdlibOutcome{refuses: true}},
		// Review finding 3: browsers end a comment at --!>.
		{name: "comment closed with bang", input: `<p><!-- a --!>b--></p>`, want: []string{
			`1:4: browsers end this comment at the "--!>" at 1:11 but html/template strips through the "-->" at 1:16, hiding the markup between`,
		}, stdlib: stdlibOutcome{has: []string{"<p></p>"}, lacks: []string{"b"}}},
		// Review finding 4: a --> in an action is not where html/template
		// ends the comment, and one in a template it calls is.
		{name: "abrupt comment past close in action", input: `<p><!-->{{"-->"}}x--></p>`, want: []string{
			`1:4: browsers end this comment at the "<!-->" at 1:4 but html/template strips through the "-->" at 1:19, hiding the markup between`,
		}, stdlib: stdlibOutcome{has: []string{"<p></p>"}}},
		{name: "abrupt comment closed in callee", input: `<p><!-->{{template "x"}}tail--></p>{{define "x"}}in-->out{{end}}`, want: []string{
			`1:4: browsers end this comment at the "<!-->" at 1:4 but html/template strips through the "-->" at 1:52, hiding the markup between`,
		}, stdlib: stdlibOutcome{has: []string{"<p>outtail--></p>"}}},
		{name: "comment closed in callee", input: `<p><!-- {{template "x"}} --></p>{{define "x"}}a-->b{{end}}`, want: []string{
			`1:4: browsers end this comment at the "-->" at 1:26 but html/template strips through the "-->" at 1:48, hiding the markup between`,
		}, stdlib: stdlibOutcome{has: []string{"<p>b --></p>"}}},
		{name: "comment around callee", input: `<p><!-- {{template "x"}} --></p>{{define "x"}}a<b>c</b>{{end}}`, stdlib: stdlibOutcome{has: []string{"<p></p>"}}},
		// Review finding 5: content browsers read as raw text is HTML
		// text to html/template.
		{name: "comment in xmp", input: `<xmp>a<!-- c -->b</xmp>`, want: []string{"1:7: " + fmt.Sprintf(rawStrips, "xmp")}, stdlib: stdlibOutcome{has: []string{"<xmp>ab</xmp>"}}},
		{name: "bang in xmp", input: `<xmp><!x></xmp>`, want: []string{"1:6: " + fmt.Sprintf(rawBang, "xmp")}, stdlib: stdlibOutcome{has: []string{"&lt;!x>"}}},
		{name: "question in plaintext", input: `<plaintext><?x>`, want: []string{"1:12: " + fmt.Sprintf(rawPI, "plaintext")}, stdlib: stdlibOutcome{has: []string{"&lt;?x>"}}},
		// noscript content is HTML with scripting off, and iframe,
		// noembed and noframes content is not rendered.
		{name: "comment in noscript", input: `<noscript><!-- c --></noscript>`, stdlib: stdlibOutcome{has: []string{"<noscript></noscript>"}}},
		{name: "bang in noscript", input: `<noscript><!x></noscript>`, want: []string{"1:11: " + bang}, stdlib: stdlibOutcome{has: []string{"&lt;!x>"}}},
		{name: "bang in iframe", input: `<iframe><!x></iframe>`, stdlib: stdlibOutcome{has: []string{"&lt;!x>"}}},
		{name: "comment in noembed", input: `<noembed><!-- c --></noembed>`, stdlib: stdlibOutcome{lacks: []string{"<!--"}}},
		{name: "bang in noframes", input: `<noframes><!x></noframes>`, stdlib: stdlibOutcome{has: []string{"&lt;!x>"}}},
		// A partial is checked as html/template escapes it where it
		// renders: one called in script, style or an attribute is escaped
		// there, as a copy.
		{name: "partial in script", input: `<script>{{template "js"}}</script>{{define "js"}}if (a <!x) {}{{end}}`, stdlib: stdlibOutcome{has: []string{"if (a <!x) {}"}}},
		{name: "partial in style", input: `<style>{{template "css"}}</style>{{define "css"}}/* <!x> */ p {}{{end}}`, stdlib: stdlibOutcome{lacks: []string{"<!x>"}}},
		{name: "partial in attribute", input: `<p title="{{template "x"}}">t</p>{{define "x"}}a<!x>b{{end}}`, stdlib: stdlibOutcome{lacks: []string{"&lt;!x"}}},
		{name: "partial in title", input: `<title>{{template "x"}}</title>{{define "x"}}T<!-- c -->{{end}}`, want: []string{"1:47: " + title}, stdlib: stdlibOutcome{has: []string{"T&lt;!-- c -->"}}},
		{name: "partial in xmp", input: `<xmp>{{template "x"}}</xmp>{{define "x"}}a<!-- c -->b{{end}}`, want: []string{"1:43: " + fmt.Sprintf(rawStrips, "xmp")}, stdlib: stdlibOutcome{has: []string{"<xmp>ab</xmp>"}}},
		{name: "partial in text", input: `<p>{{template "x"}}</p>{{define "x"}}a<!x>b{{end}}`, want: []string{"1:39: " + bang}, stdlib: stdlibOutcome{has: []string{"a&lt;!x>b"}}},
		{name: "less than in xmp", input: `<xmp>a < b</xmp>`, stdlib: stdlibOutcome{has: []string{"a &lt; b"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := slices.Clone(test.want)
			description := checkStdlib(t, test.input, test.stdlib)
			for i, w := range want {
				if strings.HasSuffix(w, refuses) {
					want[i] = w + description
				}
			}
			got := checkComments(t, test.input, []string{"test.gohtml"})
			if !slices.Equal(got, want) {
				t.Errorf("CheckComments(%q) =\n%q\nwant\n%q", test.input, got, want)
			}
		})
	}
}

// TestCheckComments_defines checks that a define body is reported once,
// in its own text, and that a define body between an abrupt comment and
// a "-->" is not where html/template ends the comment.
func TestCheckComments_defines(t *testing.T) {
	const input = `<p><!-->{{define "x"}}--><!x>{{end}} --></p>`
	checkStdlib(t, input, stdlibOutcome{has: []string{"<p></p>"}})
	want := []string{
		`1:4: browsers end this comment at the "<!-->" at 1:4 but html/template strips through the "-->" at 1:38, hiding the markup between`,
		`1:26: html/template escapes this "<!", so browsers show it as text; comment with {{/* */}} or <!-- -->`,
	}
	if got := checkComments(t, input, nil); !slices.Equal(got, want) {
		t.Errorf("CheckComments(%q) =\n%q\nwant\n%q", input, got, want)
	}
}

// TestCheckComments_roots checks that the escaper's error is reported
// for a template the program executes, and not for one only a call
// reaches, which html/template escapes in the caller's context, nor for
// any template when there are no roots.
func TestCheckComments_roots(t *testing.T) {
	const input = `{{template "open"}}/x">y</a>{{define "open"}}<a href="{{end}}`
	checkStdlib(t, input, stdlibOutcome{has: []string{`<a href="/x">y</a>`}})
	for _, roots := range [][]string{{"test.gohtml"}, nil} {
		if got := checkComments(t, input, roots); len(got) != 0 {
			t.Errorf("CheckComments(%q, %q) = %q, want none", input, roots, got)
		}
	}
}

// TestCheckComments_refusalInCallee checks that an escaper error with no
// node is reported at the start of the template, in another file, that
// the escaper stops with the same error escaping it alone.
func TestCheckComments_refusalInCallee(t *testing.T) {
	const (
		a = `<p>{{template "x"}}</p>`
		b = "{{define \"x\"}}\n<li{{if true}} title=\"a\" <!x>{{end}}>{{end}}"
	)
	std := htmltemplate.Must(htmltemplate.Must(htmltemplate.New("a.gohtml").Parse(a)).New("b.gohtml").Parse(b))
	escapeErr, ok := errors.AsType[*htmltemplate.Error](std.ExecuteTemplate(io.Discard, "a.gohtml", nil))
	if !ok {
		t.Fatalf("html/template does not refuse a.gohtml")
	}
	set := muxthtml.New("a.gohtml")
	if _, err := set.Parse(a); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if _, err := set.New("b.gohtml").Parse(b); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	diags, err := CheckComments(setTrees(set), []string{"a.gohtml"})
	if err != nil {
		t.Fatalf("CheckComments error: %v", err)
	}
	want := []string{"b.gohtml:1:15: html/template refuses this template: " + escapeErr.Description}
	var got []string
	for _, d := range diags {
		p := d.Tree.PositionAt(d.Pos)
		got = append(got, fmt.Sprintf("%s:%d:%d: %s", d.Tree.ParseName, p.Line, p.Column, d.Message))
	}
	if !slices.Equal(got, want) {
		t.Errorf("CheckComments =\n%q\nwant\n%q", got, want)
	}
}

// TestCheckComments_files checks that html/template's end of a comment in
// another file is named with the file, and that html/template parses with
// the delimiters the set was parsed with.
func TestCheckComments_files(t *testing.T) {
	set := muxthtml.New("a.gohtml").Delims("[[", "]]")
	if _, err := set.Parse(`<p><!-->[[template "x"]]--></p>`); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if _, err := set.New("b.gohtml").Parse("[[define \"x\"]]\nin-->out[[end]]"); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	diags, err := CheckComments(setTrees(set), []string{"a.gohtml"})
	if err != nil {
		t.Fatalf("CheckComments error: %v", err)
	}
	want := []string{
		`a.gohtml:1:4: browsers end this comment at the "<!-->" at 1:4 but html/template strips through the "-->" at b.gohtml:2:3, hiding the markup between`,
	}
	var got []string
	for _, d := range diags {
		p := d.Tree.PositionAt(d.Pos)
		got = append(got, fmt.Sprintf("%s:%d:%d: %s", d.Tree.ParseName, p.Line, p.Column, d.Message))
	}
	if !slices.Equal(got, want) {
		t.Errorf("CheckComments =\n%q\nwant\n%q", got, want)
	}
}

func TestCheckComments_severity(t *testing.T) {
	set, err := muxthtml.New("test.gohtml").Parse(`<p><!x></p>`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	diags, err := CheckComments(setTrees(set), nil)
	if err != nil {
		t.Fatalf("CheckComments error: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("CheckComments reported nothing")
	}
	for _, d := range diags {
		if d.Severity != SeverityError {
			t.Errorf("Severity = %v, want %v", d.Severity, SeverityError)
		}
	}
}

// checkStdlib executes input through the standard library's html/template
// and fails t unless the outcome is want. It returns the description of
// the error html/template refuses the template with.
func checkStdlib(t *testing.T, input string, want stdlibOutcome) string {
	t.Helper()
	tmpl, err := htmltemplate.New("test.gohtml").Parse(input)
	if err != nil {
		t.Fatalf("html/template Parse error: %v", err)
	}
	var out strings.Builder
	err = tmpl.Execute(&out, map[string]any{"X": true, "A": true, "T": "t", "R": []int{1}, "W": 1})
	escapeErr, refused := errors.AsType[*htmltemplate.Error](err)
	switch {
	case want.refuses && !refused:
		t.Fatalf("html/template Execute = %q, %v; want it to refuse the template", out.String(), err)
	case want.refuses:
		return escapeErr.Description
	case err != nil:
		t.Fatalf("html/template Execute error: %v", err)
	}
	for _, s := range want.has {
		if !strings.Contains(out.String(), s) {
			t.Errorf("html/template wrote %q, want it to contain %q", out.String(), s)
		}
	}
	for _, s := range want.lacks {
		if strings.Contains(out.String(), s) {
			t.Errorf("html/template wrote %q, want it to lack %q", out.String(), s)
		}
	}
	return ""
}

// checkComments parses input as the set test.gohtml and returns what
// CheckComments reports, as line:column: message.
func checkComments(t *testing.T, input string, roots []string) []string {
	t.Helper()
	set, err := muxthtml.New("test.gohtml").Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	diags, err := CheckComments(setTrees(set), roots)
	if err != nil {
		t.Fatalf("CheckComments error: %v", err)
	}
	var lines []string
	for _, d := range diags {
		p := d.Tree.PositionAt(d.Pos)
		lines = append(lines, fmt.Sprintf("%d:%d: %s", p.Line, p.Column, d.Message))
	}
	return lines
}

func setTrees(set *muxthtml.Template) map[string]*template.Tree {
	trees := make(map[string]*template.Tree)
	for _, t := range set.Templates() {
		if t.Tree != nil {
			trees[t.Name()] = t.Tree
		}
	}
	return trees
}

// TestCheckComments_stopsAtFirstWrite checks that escaping a template
// does not run it: html/template escapes before it writes, and the
// escaper's writer fails at the first write.
func TestCheckComments_stopsAtFirstWrite(t *testing.T) {
	if got := checkComments(t, `<p>{{range 1000000000}}<!x>{{end}}</p>`, []string{"test.gohtml"}); len(got) != 1 {
		t.Errorf("CheckComments = %q, want the escaped <!", got)
	}
}
