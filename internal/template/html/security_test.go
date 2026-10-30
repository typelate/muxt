package html_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/html"
	"github.com/typelate/muxt/internal/template/lex"
)

// render parses tmpl and returns the rendered output.
func render(t *testing.T, tmpl string, data any) string {
	t.Helper()
	tt, err := html.New("test").Parse(tmpl)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, tt.Execute(&buf, data))
	return buf.String()
}

// --- Trust-type honoring ---

func TestTrustHTMLBypassesBodyEscape(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{"trusted HTML renders raw", html.HTML(`<b>safe</b>`), `<div><b>safe</b></div>`},
		{"untrusted string escapes", `<b>not safe</b>`, `<div>&lt;b&gt;not safe&lt;/b&gt;</div>`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := render(t, `<div>{{.}}</div>`, tc.data)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTrustURLBypassesSchemeFilter(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{"trusted javascript URL passes", html.URL(`javascript:onSafeAction()`),
			`<a href="javascript:onSafeAction()">x</a>`},
		{"trusted custom scheme passes", html.URL(`tel:+15551234`),
			`<a href="tel:+15551234">x</a>`},
		{"untrusted javascript URL filtered", `javascript:alert(1)`,
			`<a href="#ZgotmplZ">x</a>`},
		{"untrusted tel URL filtered", `tel:+15551234`,
			`<a href="#ZgotmplZ">x</a>`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := render(t, `<a href="{{.}}">x</a>`, tc.data)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTrustJSInScriptBody(t *testing.T) {
	got := render(t, `<script>var x = {{.}};</script>`, html.JS(`makeWidget("hi")`))
	assert.Equal(t, `<script>var x = makeWidget("hi");</script>`, got)
}

func TestTrustJSStrInsideStringLiteral(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{"trusted JSStr verbatim", html.JSStr(`already-escaped`),
			`<script>var x = "already-escaped";</script>`},
		{"untrusted string in string-literal context is JS-escaped",
			`</script>`,
			"<script>var x = \"\\u003c\\/script\\u003e\";</script>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := render(t, `<script>var x = "{{.}}";</script>`, tc.data)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTrustCSSInStyleBody(t *testing.T) {
	t.Run("trusted CSS passes through", func(t *testing.T) {
		got := render(t, `<style>{{.}}</style>`,
			html.CSS(`body { color: red; }`))
		assert.Equal(t, `<style>body { color: red; }</style>`, got)
	})
	t.Run("untrusted string cannot inject rules", func(t *testing.T) {
		got := render(t, `<style>.theme-{{.}} { color: red; }</style>`,
			`x } body { background: black; `)
		assert.Contains(t, got, "<style>")
		assert.Contains(t, got, "\\7b ", "{ should be CSS-escaped")
		assert.Contains(t, got, "\\7d ", "} should be CSS-escaped")
		assert.NotContains(t, got, "} body {")
	})
}

func TestTrustHTMLAttrBypassesContextualEscape(t *testing.T) {
	t.Run("HTMLAttr in href skips URL filter", func(t *testing.T) {
		got := render(t, `<a href="{{.}}">x</a>`,
			html.HTMLAttr(`javascript:trustedCall()`))
		assert.Equal(t, `<a href="javascript:trustedCall()">x</a>`, got)
	})
	t.Run("HTMLAttr in onclick skips JS-event escape", func(t *testing.T) {
		got := render(t, `<button onclick="{{.}}">x</button>`,
			html.HTMLAttr(`callMe()`))
		assert.Equal(t, `<button onclick="callMe()">x</button>`, got)
	})
}

func TestTrustSrcsetBypassesPerURLFilter(t *testing.T) {
	t.Run("trusted srcset passes", func(t *testing.T) {
		got := render(t, `<img srcset="{{.}}">`,
			html.Srcset(`javascript:custom() 1x, https://x/y 2x`))
		assert.Equal(t,
			`<img srcset="javascript:custom() 1x, https://x/y 2x">`, got)
	})
	t.Run("untrusted srcset gets per-URL filtering", func(t *testing.T) {
		got := render(t, `<img srcset="{{.}}">`,
			`javascript:alert(1) 1x, https://x/y 2x`)
		assert.Contains(t, got, `#ZgotmplZ 1x`)
		assert.Contains(t, got, `https://x/y 2x`)
	})
}

// --- New behavior: JS expression context ---

func TestJSExpressionContextNeutralizesInjection(t *testing.T) {
	got := render(t, `<script>var x = {{.}};</script>`, `1; alert(1)`)
	assert.Equal(t, `<script>var x = "1; alert(1)";</script>`, got)
	assert.NotContains(t, got, `var x = 1; alert(1);`)
}

func TestJSStringContextEscapeStaysCompatible(t *testing.T) {
	got := render(t, `<script>var x = "{{.}}";</script>`, `</script>`)
	assert.Equal(t, "<script>var x = \"\\u003c\\/script\\u003e\";</script>", got)
}

// --- New behavior: CSS context escape table ---

func TestCSSContextBlocksRuleBreakout(t *testing.T) {
	got := render(t, `<style>.theme-{{.}} { color: red; }</style>`,
		`x } body { background: black`)
	assert.NotContains(t, got, "} body {", "rule breakout must not be possible")
	assert.Contains(t, got, "\\7d ", "} must be CSS-hex-escaped")
}

// --- Dynamic attribute names parse as text ---

// TestDynamicAttributeNameParses checks that a template with an action in
// attribute-name position parses, as it does with html/template: the
// parser reads the tag as text with actions. Execute does not escape such
// an action for the attribute-name context; that is one of the escaping
// gaps documented on Template.Execute, which is why Execute is not for
// serving responses.
func TestDynamicAttributeNameParses(t *testing.T) {
	_, err := html.New("test").Parse(`<div {{.Name}}="{{.Value}}">x</div>`)
	require.NoError(t, err)
}

// --- New behavior: HTMLMode forced for shim trees ---

func TestAddParseTreeForcesHTMLMode(t *testing.T) {
	src := `<p>{{.}}</p>`
	parsed, err := template.ParseTree("plain", "plain", src, nil, lex.Options{})
	require.NoError(t, err)

	tmpl := html.New("main")
	_, err = tmpl.AddParseTree("plain", parsed)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, tmpl.ExecuteTemplate(&buf, "plain",
		`<script>alert(1)</script>`))
	got := buf.String()
	assert.NotContains(t, got, "<script>", "AddParseTree must HTML-escape")
	assert.Contains(t, got, "&lt;script&gt;")
}

// --- New behavior: full set of JS event handlers escaped ---

func TestExtendedJSEventHandlersEscaped(t *testing.T) {
	for _, attr := range []string{"onpopstate", "onhashchange", "ontouchstart", "onmessage"} {
		t.Run(attr, func(t *testing.T) {
			tmpl := `<div ` + attr + `="{{.}}">x</div>`
			got := render(t, tmpl, `";alert(1);//`)
			assert.Contains(t, got, `\u0022`,
				"%s should be JS-event-escaped", attr)
		})
	}
}

func TestNonEventOnPrefixAttributeNotEscaped(t *testing.T) {
	got := render(t, `<div only="{{.}}">x</div>`, `he said "hi"`)
	assert.NotContains(t, got, `\u0022`,
		"non-event 'only' attr should not be JS-escaped")
	assert.Contains(t, got, `&#34;`,
		"non-event 'only' attr should be HTML-attr-escaped only")
}

// --- New behavior: srcset URL filtering ---

func TestSrcsetBlocksDangerousURLs(t *testing.T) {
	got := render(t, `<img srcset="{{.}}">`,
		`javascript:alert(1) 1x, https://example/x.png 2x`)
	assert.Contains(t, got, "#ZgotmplZ 1x")
	assert.Contains(t, got, "https://example/x.png 2x")
}

// --- Regression: existing happy paths still work ---

func TestUnchangedDefaultEscaping(t *testing.T) {
	tests := []struct {
		name, tmpl string
		data       any
		want       string
	}{
		{"text body escapes script", `<p>{{.}}</p>`,
			`<script>alert(1)</script>`,
			`<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>`},
		{"href filter blocks javascript:", `<a href="{{.}}">x</a>`,
			`javascript:alert(1)`,
			`<a href="#ZgotmplZ">x</a>`},
		{"onclick value JS-escaped",
			`<button onclick="{{.}}">x</button>`,
			`alert("xss")`,
			"<button onclick=\"&#34;alert(\\u0022xss\\u0022)&#34;\">x</button>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, render(t, tc.tmpl, tc.data))
		})
	}
}

func TestTrustNoopInWrongContext(t *testing.T) {
	t.Run("URL in body is HTML-escaped", func(t *testing.T) {
		got := render(t, `<p>{{.}}</p>`, html.URL(`<script>`))
		assert.Equal(t, `<p>&lt;script&gt;</p>`, got)
	})
	t.Run("HTML trust does not bypass URL filter in href", func(t *testing.T) {
		got := render(t, `<a href="{{.}}">x</a>`,
			html.HTML(`javascript:alert(1)`))
		assert.NotContains(t, got, `href="javascript:alert(1)"`,
			"HTML trust must not bypass URL filter")
	})
}
