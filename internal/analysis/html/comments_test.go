package analysis

import (
	"fmt"
	"slices"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCheckComments(t *testing.T) {
	const (
		bang  = `html/template escapes this "<!", so browsers show it as text; comment with {{/* */}} or <!-- -->`
		title = `html/template writes this <!-- as text inside <title>; browsers show it`
		area  = `html/template writes this <!-- as text inside <textarea>; browsers show it`
	)
	for _, test := range []struct {
		name, input string
		want        []string
	}{
		{name: "comment in text", input: `<p>a<!-- c -->b</p>`},
		{name: "action in comment", input: `<p><!-- {{.X}} --></p>`},
		{name: "control in comment", input: `<p><!-- a {{if .X}}b{{end}} --></p>`},
		{name: "doctype", input: "<!DOCTYPE html>\n<html></html>"},
		{name: "lower case doctype", input: `<!doctype html>`},
		{name: "comment in attribute", input: `<p title="<!-- c -->">x</p>`},
		{name: "bang in attribute", input: `<p title="<!x>">x</p>`},
		{name: "comment in script", input: `<script>/*<!-- c -->*/</script>`},
		{name: "bang in script", input: `<script>if (a <!x) {}</script>`},
		{name: "comment in style", input: `<style><!-- c --></style>`},
		{name: "text in title", input: `<title>a < b</title>`},
		{name: "empty comment", input: `<p><!----></p>`},

		{name: "comment in title", input: `<title>T<!-- c --></title>`, want: []string{"1:9: " + title}},
		{name: "comment in textarea", input: `<TEXTAREA>T<!-- c --></TEXTAREA>`, want: []string{"1:12: " + area}},
		{name: "comment after action in title", input: "<title>{{.T}}\n<!-- c --></title>", want: []string{"2:1: " + title}},
		{name: "comment in svg title", input: `<svg><title><!-- c --></title></svg>`, want: []string{"1:13: " + title}},

		{name: "bogus comment", input: `<p><!x></p>`, want: []string{"1:4: " + bang}},
		{name: "processing instruction", input: `<p><?x></p>`, want: []string{"1:4: " + bang}},
		{name: "conditional comment", input: "<div>\n  <![if !IE]>a<![endif]>\n</div>", want: []string{"2:3: " + bang, "2:15: " + bang}},
		{name: "bang before action", input: `<p><!{{.X}}></p>`, want: []string{"1:4: " + bang}},
		{name: "bang in branch", input: `<p>{{if .X}}<!{{end}}x></p>`, want: []string{"1:13: " + bang}},

		{name: "abrupt comment", input: "<p><!-->a\n<b>x</b> --></p>", want: []string{
			`1:4: browsers end this comment at "<!-->" but html/template strips through the next "-->" at 2:10, hiding the markup between`,
		}},
		{name: "abrupt comment with dash", input: `<p><!--->a <b>x</b>--></p>`, want: []string{
			`1:4: browsers end this comment at "<!--->" but html/template strips through the next "-->" at 1:20, hiding the markup between`,
		}},
		{name: "abrupt comment never closed", input: `<p><!-->a</p>`, want: []string{
			`1:4: browsers end this comment at "<!-->" but html/template strips through the next "-->", and the comment never closes`,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}
			got := formatDiagnostics(tree, CheckComments(tree))
			if !slices.Equal(got, test.want) {
				t.Errorf("CheckComments(%q) =\n%q\nwant\n%q", test.input, got, test.want)
			}
		})
	}
}

// TestCheckComments_defines checks that a define body is reported in the
// define's own tree, not again in the file tree that holds it, and that a
// define body between an abrupt comment and a "-->" is not where
// html/template ends the comment.
func TestCheckComments_defines(t *testing.T) {
	const input = `<p><!-->{{define "x"}}--><!x>{{end}} --></p>`
	treeSet := make(map[string]*template.Tree)
	if _, err := template.ParseTree("test.gohtml", "test.gohtml", input, treeSet, lex.NewOptions()); err != nil {
		t.Fatalf("ParseTree error: %v", err)
	}
	for _, test := range []struct {
		name string
		want []string
	}{
		{"test.gohtml", []string{
			`1:4: browsers end this comment at "<!-->" but html/template strips through the next "-->" at 1:38, hiding the markup between`,
		}},
		{"x", []string{
			`1:26: html/template escapes this "<!", so browsers show it as text; comment with {{/* */}} or <!-- -->`,
		}},
	} {
		tree := treeSet[test.name]
		if tree == nil {
			t.Fatalf("no tree %q", test.name)
		}
		got := formatDiagnostics(tree, CheckComments(tree))
		if !slices.Equal(got, test.want) {
			t.Errorf("CheckComments(%q) =\n%q\nwant\n%q", test.name, got, test.want)
		}
	}
}

func TestCheckComments_severity(t *testing.T) {
	tree, err := template.ParseTree("test.gohtml", t.Name(), `<p><!x></p>`, nil, lex.NewOptions())
	if err != nil {
		t.Fatalf("ParseTree error: %v", err)
	}
	for _, d := range CheckComments(tree) {
		if d.Severity != SeverityError {
			t.Errorf("Severity = %v, want %v", d.Severity, SeverityError)
		}
	}
}

func formatDiagnostics(tree *template.Tree, diags Diagnostics) []string {
	var lines []string
	for _, d := range diags {
		p := tree.PositionAt(d.Pos)
		lines = append(lines, fmt.Sprintf("%d:%d: %s", p.Line, p.Column, d.Message))
	}
	return lines
}
