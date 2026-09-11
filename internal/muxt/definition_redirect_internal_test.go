package muxt

import (
	"html/template"
	"testing"
)

// pageIn parses text as a set holding "page", plus whatever else the text
// defines, and returns the set with the root of "page".
func pageIn(t *testing.T, text string) (*template.Template, *template.Template) {
	t.Helper()
	ts, err := template.New("set").Parse(`{{define "page"}}` + text + `{{end}}`)
	if err != nil {
		t.Fatal(err)
	}
	page := ts.Lookup("page")
	if page == nil || page.Tree == nil {
		t.Fatalf("the set does not hold a parsed page: %q", text)
	}
	return ts, page
}

// TestWritesResponseState states which TemplateData methods reach the
// client only through the status line muxt writes, which is what makes
// them worth a diagnostic when muxt is not the one writing it.
func TestWritesResponseState(t *testing.T) {
	for _, tt := range []struct {
		method string
		want   bool
	}{
		{method: "StatusCode", want: true},
		{method: "Redirect", want: true},
		{method: "RedirectFound", want: true},
		{method: "RedirectSeeOther", want: true},
		{method: "RedirectMovedPermanently", want: true},
		{method: "RedirectMultipleChoices", want: true},
		// Header writes to the response header map directly, so it works
		// whoever owns the response.
		{method: "Header"},
		{method: "Name"},
	} {
		if got := writesResponseState(tt.method); got != tt.want {
			t.Errorf("writesResponseState(%q) = %t, want %t", tt.method, got, tt.want)
		}
	}
}

// TestFindResponseStateCall states when a template is reported as writing
// response state: the call has to be on the TemplateData the handler
// passed in. Inside a with or a range, .StatusCode names a field of
// whatever was selected, and reporting that would reject a working route.
func TestFindResponseStateCall(t *testing.T) {
	for _, tt := range []struct {
		name     string
		template string
		want     string
	}{
		{name: "a status code", template: `{{.StatusCode 201}}`, want: "StatusCode"},
		{name: "a redirect", template: `{{.Redirect "/x"}}`, want: "Redirect"},
		{name: "another redirect", template: `{{.RedirectSeeOther "/x"}}`, want: "RedirectSeeOther"},
		{name: "a header, which muxt does not write", template: `{{.Header.Set "k" "v"}}`},
		{name: "nothing at all", template: `<p>hi</p>`},

		{name: "an if does not rebind dot", template: `{{if .Loud}}{{.StatusCode 201}}{{end}}`, want: "StatusCode"},
		{name: "a with rebinds dot in its body", template: `{{with .Inner}}{{.StatusCode 201}}{{end}}`},
		{name: "a range rebinds dot in its body", template: `{{range .Items}}{{.StatusCode 201}}{{end}}`},
		{
			name:     "an else runs with the dot outside the with",
			template: `{{with .Inner}}x{{else}}{{.StatusCode 201}}{{end}}`,
			want:     "StatusCode",
		},
		{
			name:     "the dollar is the dot the template started with",
			template: `{{range .Items}}{{$.StatusCode 201}}{{end}}`,
			want:     "StatusCode",
		},
		{
			name:     "a parenthesised call is walked on its own",
			template: `{{(.Redirect "/x").Header}}`,
			want:     "Redirect",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, page := pageIn(t, tt.template)
			got, ok := findResponseStateCall(page.Tree.Root, ts)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("findResponseStateCall(%s) = %q, %t, want %q, %t", tt.template, got, ok, tt.want, tt.want != "")
			}
		})
	}
}

// TestFindResponseStateCallThroughTemplateCalls states that the report
// follows a {{template}} invocation only as far as the dot goes: a called
// template receives TemplateData only when the invocation passes dot along
// whole.
func TestFindResponseStateCallThroughTemplateCalls(t *testing.T) {
	for _, tt := range []struct {
		name string
		call string
		want string
	}{
		{name: "dot passed along whole", call: `{{template "part" .}}`, want: "StatusCode"},
		{name: "a value selected out of dot", call: `{{template "part" .Inner}}`},
		{name: "no argument at all", call: `{{template "part"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := template.New("set").Parse(
				`{{define "page"}}` + tt.call + `{{end}}{{define "part"}}{{.StatusCode 201}}{{end}}`)
			if err != nil {
				t.Fatal(err)
			}
			page := ts.Lookup("page")
			got, ok := findResponseStateCall(page.Tree.Root, ts)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("findResponseStateCall(%s) = %q, %t, want %q, %t", tt.call, got, ok, tt.want, tt.want != "")
			}
		})
	}
}

// TestCanTemplateRedirect states when the generated handler gets its
// redirect block. The answer is deliberately conservative: it does not
// care whether dot was rebound, and passing dot to a function counts,
// because emitting the block for a template that never redirects costs
// nothing while omitting it for one that does is a bug.
func TestCanTemplateRedirect(t *testing.T) {
	for _, tt := range []struct {
		name     string
		template string
		want     bool
	}{
		{name: "a redirect", template: `{{.Redirect "/x"}}`, want: true},
		{name: "another redirect", template: `{{.RedirectFound "/x"}}`, want: true},
		{name: "inside a branch", template: `{{if .Loud}}{{.Redirect "/x"}}{{end}}`, want: true},
		{name: "inside a range, where dot is not TemplateData", template: `{{range .Items}}{{.Redirect "/x"}}{{end}}`, want: true},
		{name: "dot handed to a function", template: `{{printf "%v" .}}`, want: true},
		{name: "static text", template: `<p>hi</p>`},
		{name: "a literal condition", template: `{{if true}}x{{end}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, page := pageIn(t, tt.template)
			if got := canTemplateRedirect(page.Tree.Root, ts, make(map[string]bool)); got != tt.want {
				t.Errorf("canTemplateRedirect(%s) = %t, want %t", tt.template, got, tt.want)
			}
		})
	}
}

// TestCanTemplateRedirectFollowsTemplateCalls states that a partial's
// redirect is the calling route's redirect, and that a template reaching
// itself is walked once rather than for ever.
func TestCanTemplateRedirectFollowsTemplateCalls(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  string
		want bool
	}{
		{
			name: "a partial that redirects",
			set:  `{{define "page"}}{{template "part" .}}{{end}}{{define "part"}}{{.Redirect "/x"}}{{end}}`,
			want: true,
		},
		{
			name: "a partial that does not",
			set:  `{{define "page"}}{{template "part" .}}{{end}}{{define "part"}}<p>hi</p>{{end}}`,
		},
		{
			name: "a template that calls itself",
			set:  `{{define "page"}}{{template "page" .}}<p>hi</p>{{end}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := template.New("set").Parse(tt.set)
			if err != nil {
				t.Fatal(err)
			}
			page := ts.Lookup("page")
			if got := canTemplateRedirect(page.Tree.Root, ts, make(map[string]bool)); got != tt.want {
				t.Errorf("canTemplateRedirect = %t, want %t", got, tt.want)
			}
		})
	}
}
