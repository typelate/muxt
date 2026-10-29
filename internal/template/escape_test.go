package template_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/template"
)

// TestEscapeJS exercises the public EscapeJS function against each of the
// individual characters/sequences it must rewrite. Cases are organised by
// what the escape prevents: control characters, quote boundaries, HTML
// sigils, and UTF-7 attacks.
func TestEscapeJS(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// trivial pass-throughs
		{"empty", "", ""},
		{"plain ascii", "foo", "foo"},

		// control characters
		{"null byte", "\u0000", `\u0000`},
		{"tab", "\t", `\t`},
		{"newline", "\n", `\n`},
		{"carriage return", "\r", `\r`},
		{"line separator U+2028", "\u2028", `\u2028`},
		{"paragraph separator U+2029", "\u2029", `\u2029`},

		// backslashes
		{"single backslash", `\`, `\\`},
		{"backslash n", `\n`, `\\n`},
		{"CRLF between text", "foo\r\nbar", `foo\r\nbar`},

		// preserve attribute boundaries — quotes escape to numeric form
		{"double quote", `"`, `\u0022`},
		{"single quote", `'`, `\u0027`},

		// allow embedding in HTML without further escaping
		{"ampersand entity", `&amp;`, `\u0026amp;`},

		// prevent breaking out of script / CDATA / comment
		{"script close tag", "</script>", `\u003c\/script\u003e`},
		{"CDATA open", "<![CDATA[", `\u003c![CDATA[`},
		{"CDATA close", "]]>", `]]\u003e`},
		{"HTML comment open", "<!--", `\u003c!--`},
		{"HTML comment close", "-->", `--\u003e`},

		// UTF-7 injection
		{
			name: "UTF-7 attack vector",
			in:   "+ADw-script+AD4-alert(1)+ADw-/script+AD4-",
			want: `\u002bADw-script\u002bAD4-alert(1)\u002bADw-\/script\u002bAD4-`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, template.EscapeJS(tc.in), "EscapeJS(%q)", tc.in)
		})
	}
}

// TestEscapeJSFullRange verifies EscapeJS against every byte in [0,127] plus
// select high codepoints, then re-runs rune-by-rune to ensure the fast-path
// for strings agrees with per-rune escaping.
func TestEscapeJSFullRange(t *testing.T) {
	input := "\x00\x01\x02\x03\x04\x05\x06\x07\x08\t\n\x0b\x0c\r\x0e\x0f" +
		"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f" +
		` !"#$%&'()*+,-./` +
		`0123456789:;<=>?` +
		`@ABCDEFGHIJKLMNO` +
		`PQRSTUVWXYZ[\]^_` +
		"`abcdefghijklmno" +
		"pqrstuvwxyz{|}~\x7f" +
		"\u00A0\u0100\u2028\u2029\ufeff\U0001D11E"

	want := `\u0000\u0001\u0002\u0003\u0004\u0005\u0006\u0007` +
		`\u0008\t\n\u000b\f\r\u000e\u000f` +
		`\u0010\u0011\u0012\u0013\u0014\u0015\u0016\u0017` +
		`\u0018\u0019\u001a\u001b\u001c\u001d\u001e\u001f` +
		` !\u0022#$%\u0026\u0027()*\u002b,-.\/` +
		`0123456789:;\u003c=\u003e?` +
		`@ABCDEFGHIJKLMNO` +
		`PQRSTUVWXYZ[\\]^_` +
		"\\u0060abcdefghijklmno" +
		"pqrstuvwxyz{|}~\u007f" +
		"\u00A0\u0100\\u2028\\u2029\ufeff\U0001D11E"

	assert.Equal(t, want, template.EscapeJS(input), "EscapeJS whole input")

	var buf strings.Builder
	for _, c := range input {
		buf.WriteString(template.EscapeJS(string(c)))
	}
	assert.Equal(t, want, buf.String(), "EscapeJS rune-by-rune agrees with whole-string")
}

// TestFilterURL exercises the URL scheme allowlist used by context-aware
// escaping. Dangerous schemes are replaced by the sentinel "#ZgotmplZ"; safe
// and scheme-less URLs pass through unchanged (modulo whitespace trimming).
func TestFilterURL(t *testing.T) {
	const blocked = "#ZgotmplZ"
	cases := []struct {
		name string
		in   string
		want string
	}{
		// safe schemes
		{"http scheme", "http://example.com", "http://example.com"},
		{"https scheme", "https://example.com", "https://example.com"},
		{"HTTP uppercase", "HTTP://example.com", "HTTP://example.com"},
		{"mailto scheme", "mailto:user@example.com", "mailto:user@example.com"},

		// scheme-less URLs are safe
		{"absolute path", "/foo/bar", "/foo/bar"},
		{"relative path", "foo/bar", "foo/bar"},
		{"fragment only", "#fragment", "#fragment"},
		{"query only", "?query=1", "?query=1"},

		// dangerous schemes get replaced
		{"javascript", "javascript:alert(1)", blocked},
		{"JavaScript mixed case", "JavaScript:alert(1)", blocked},
		{"vbscript", "vbscript:MsgBox", blocked},
		{"data scheme", "data:text/html,<script>alert(1)</script>", blocked},

		// tel and ftp match html/template's allowlist — blocked
		{"tel scheme blocked", "tel:+1234567890", blocked},
		{"ftp scheme blocked", "ftp://example.com", blocked},

		// whitespace is trimmed before scheme detection
		{"pad around javascript", "  javascript:alert(1)  ", blocked},
		{"pad around https", "  https://example.com  ", "https://example.com"},

		// a ':' after a '/' is path text, not a scheme
		{"colon in absolute path", "/foo:bar", "/foo:bar"},
		{"colon in relative path", "foo/bar:baz", "foo/bar:baz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, template.FilterURL(tc.in), "FilterURL(%q)", tc.in)
		})
	}
}

// autoEscapeCase describes a context-aware escaping scenario: a template, a
// data value, and the expected rendered output. A zero-valued wantText means
// "should render as empty string" (used by the HTML-comment cases).
type autoEscapeCase struct {
	name     string
	template string
	data     any
	want     string
}

// TestAutoEscape exercises the context-aware escaper across every place an
// action can appear: text, attribute value, URL attribute, comment, JS event
// handler, and explicit escape functions in a pipeline.
func TestAutoEscape(t *testing.T) {
	groups := []struct {
		kind  string
		cases []autoEscapeCase
	}{
		{
			kind: "text context",
			cases: []autoEscapeCase{
				{
					name:     "script tag in body gets HTML-escaped",
					template: `<p>{{.}}</p>`,
					data:     `<script>alert("xss")</script>`,
					want:     `<p>&lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;</p>`,
				},
				{
					name:     "HTML metacharacters all get escaped",
					template: `<p>{{.}}</p>`,
					data:     `&<>"'`,
					want:     `<p>&amp;&lt;&gt;&#34;&#39;</p>`,
				},
				{
					name:     "null bytes preserved in text",
					template: `<p>{{.}}</p>`,
					data:     "hello\x00world",
					want:     "<p>hello\x00world</p>",
				},
			},
		},
		{
			kind: "attribute value context",
			cases: []autoEscapeCase{
				{
					name:     "quote-break attempt in href",
					template: `<a href="{{.}}">link</a>`,
					data:     `"><script>alert(1)</script>`,
					want:     `<a href="&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;">link</a>`,
				},
				{
					name:     "quote-break attempt in class",
					template: `<div class="{{.}}">x</div>`,
					data:     `" onclick="alert(1)`,
					want:     `<div class="&#34; onclick=&#34;alert(1)">x</div>`,
				},
				{
					name:     "unquoted attr gets quoted on output",
					template: `<div class={{.}}>x</div>`,
					data:     `foo onclick=alert(1)`,
					want:     `<div class="foo onclick=alert(1)">x</div>`,
				},
			},
		},
		{
			kind: "URL attribute context",
			cases: []autoEscapeCase{
				{
					name:     "safe https URL passes through",
					template: `<a href="{{.}}">link</a>`,
					data:     `https://example.com`,
					want:     `<a href="https://example.com">link</a>`,
				},
				{
					name:     "javascript scheme blocked in href",
					template: `<a href="{{.}}">link</a>`,
					data:     `javascript:alert(1)`,
					want:     `<a href="#ZgotmplZ">link</a>`,
				},
				{
					name:     "data scheme blocked in img src",
					template: `<img src="{{.}}">`,
					data:     `data:text/html,<script>alert(1)</script>`,
					want:     `<img src="#ZgotmplZ">`,
				},
				{
					name:     "javascript scheme blocked in iframe src",
					template: `<iframe src="{{.}}"></iframe>`,
					data:     `javascript:alert(document.cookie)`,
					want:     `<iframe src="#ZgotmplZ"></iframe>`,
				},
				{
					name:     "javascript scheme blocked in form action",
					template: `<form action="{{.}}"><input></form>`,
					data:     `javascript:void(0)`,
					want:     `<form action="#ZgotmplZ"><input></form>`,
				},
			},
		},
		{
			kind: "HTML comment context (entire comment is dropped)",
			cases: []autoEscapeCase{
				{"plain text comment", `<!-- {{.}} -->`, "hello world", ""},
				{"HTML inside comment", `<!-- {{.}} -->`, "<b>bold</b>", ""},
				{"comment break-out attempt", `<!-- {{.}} -->`, `--><script>alert(1)</script><!--`, ""},
				{"just dashes", `<!-- {{.}} -->`, "DASHES", ""},
			},
		},
		{
			kind: "inside <script> tag (auto JS-escape)",
			cases: []autoEscapeCase{
				{
					name:     "expression-position action is JSON-quoted",
					template: `<script>var x = {{.}};</script>`,
					data:     `"hello"`,
					want:     `<script>var x = "\u0022hello\u0022";</script>`,
				},
				{
					name:     "expression-position semicolon injection is neutralized",
					template: `<script>var x = {{.}};</script>`,
					data:     `1; alert(1)`,
					want:     `<script>var x = "1; alert(1)";</script>`,
				},
				{
					name:     "string-position action is JS-escaped without re-quoting",
					template: `<script>var x = "{{.}}";</script>`,
					data:     `</script>`,
					want:     `<script>var x = "\u003c\/script\u003e";</script>`,
				},
				{
					name:     "script close tag in expression position is JSON-quoted",
					template: `<script>var x = {{.}};</script>`,
					data:     `</script>`,
					want:     `<script>var x = "\u003c\/script\u003e";</script>`,
				},
				{
					// Regression: a struct in expression position must
					// render as a JSON object literal (json.Marshal),
					// matching html/template's jsValEscaper, not as Go's
					// %v syntax wrapped in a JS string — which is not
					// even valid JS for a composite value.
					name:     "struct in expression position is JSON-marshaled",
					template: `<script>var x = {{.}};</script>`,
					data:     struct{ Name string }{Name: "PEACH"},
					want:     `<script>var x = {"Name":"PEACH"};</script>`,
				},
				{
					name:     "slice in expression position is JSON-marshaled",
					template: `<script>var x = {{.}};</script>`,
					data:     []int{7, 14, 21},
					want:     `<script>var x = [7,14,21];</script>`,
				},
			},
		},
		{
			kind: "inside a JS template literal",
			cases: []autoEscapeCase{
				{
					name:     "interpolation sequence cannot be introduced",
					template: "<script>var a = `{{.}}`;</script>",
					data:     `${alert(1)}`,
					want:     "<script>var a = `\\u0024\\u007balert(1)\\u007d`;</script>",
				},
				{
					name:     "backtick cannot close the literal",
					template: "<script>var a = `{{.}}`;</script>",
					data:     "`;alert(1);`",
					want:     "<script>var a = `\\u0060;alert(1);\\u0060`;</script>",
				},
				{
					name:     "bare braces are escaped",
					template: "<script>var a = `{{.}}`;</script>",
					data:     `a{b}c`,
					want:     "<script>var a = `a\\u007bb\\u007dc`;</script>",
				},
				{
					name:     "ordinary text is unchanged",
					template: "<script>var a = `{{.}}`;</script>",
					data:     `hello world`,
					want:     "<script>var a = `hello world`;</script>",
				},
				{
					name:     "action inside an interpolation is quoted as an expression",
					template: "<script>var a = `x${ {{.}} }y`;</script>",
					data:     `1} ${alert(1)} ${1`,
					want:     "<script>var a = `x${ \"1} ${alert(1)} ${1\" }y`;</script>",
				},
				{
					name:     "action after an interpolation closes is literal-escaped again",
					template: "<script>var a = `${ n } {{.}}`;</script>",
					data:     `${alert(1)}`,
					want:     "<script>var a = `${ n } \\u0024\\u007balert(1)\\u007d`;</script>",
				},
			},
		},
		{
			kind: "JS event handler context",
			cases: []autoEscapeCase{
				{
					name:     "quote injection in onclick",
					template: `<button onclick="{{.}}">click</button>`,
					data:     `alert("xss")`,
					want:     `<button onclick="&#34;alert(\u0022xss\u0022)&#34;">click</button>`,
				},
				{
					name:     "control chars escaped in onclick",
					template: `<div onclick="{{.}}">x</div>`,
					data:     "\x00\t\n",
					want:     `<div onclick="&#34;\u0000\t\n&#34;">x</div>`,
				},
				{
					name:     "U+2028/U+2029 escaped in onclick",
					template: `<button onclick="{{.}}">x</button>`,
					data:     "line\u2028sep\u2029",
					want:     `<button onclick="&#34;line\u2028sep\u2029&#34;">x</button>`,
				},
			},
		},
		{
			kind: "explicit pipeline escape functions",
			cases: []autoEscapeCase{
				{
					name:     "{{html .}} in text context applies html twice",
					template: `<p>{{html .}}</p>`,
					data:     `<img src=x onerror=alert(1)>`,
					want:     `<p>&amp;lt;img src=x onerror=alert(1)&amp;gt;</p>`,
				},
				{
					name:     "{{html .}} in attr context double-escapes",
					template: `<div title="{{html .}}">x</div>`,
					data:     `"><script>alert(1)</script>`,
					want:     `<div title="&amp;#34;&amp;gt;&amp;lt;script&amp;gt;alert(1)&amp;lt;/script&amp;gt;">x</div>`,
				},
				{
					name:     "{{js .}} in text context applies js escaping",
					template: `<p>{{js .}}</p>`,
					data:     `</p><script>alert(1)</script>`,
					want:     `<p>\x3C/p\x3E\x3Cscript\x3Ealert(1)\x3C/script\x3E</p>`,
				},
				{
					name:     "{{js .}} in event handler double-escapes",
					template: `<button onclick="{{js .}}">x</button>`,
					data:     `";alert(1);//`,
					want:     `<button onclick="&#34;\\\u0022;alert(1);\/\/&#34;">x</button>`,
				},
				{
					name:     "{{urlquery .}} in text context",
					template: `<p>{{urlquery .}}</p>`,
					data:     `<script>alert(1)</script>`,
					want:     `<p>%3Cscript%3Ealert%281%29%3C%2Fscript%3E</p>`,
				},
				{
					name:     "{{urlquery .}} in URL query string",
					template: `<a href="/search?q={{urlquery .}}">search</a>`,
					data:     `<script>alert(1)</script>`,
					want:     `<a href="/search?q=%3Cscript%3Ealert%281%29%3C%2Fscript%3E">search</a>`,
				},
			},
		},
	}

	for _, g := range groups {
		t.Run(g.kind, func(t *testing.T) {
			for _, tc := range g.cases {
				t.Run(tc.name, func(t *testing.T) {
					got := execHTML(t, "test", tc.template, tc.data)
					assert.Equal(t, tc.want, got)
				})
			}
		})
	}
}

// TestAutoEscapeTextTemplate confirms that text templates bypass HTML
// escaping entirely.
func TestAutoEscapeTextTemplate(t *testing.T) {
	got := execText(t, "test", `{{.}}`, `<b>bold</b>`)
	assert.Equal(t, `<b>bold</b>`, got)
}

// TestAutoEscapeMultiContext shows the same data rendered differently in three
// contexts in a single template — the escaper picks the right rule per site.
func TestAutoEscapeMultiContext(t *testing.T) {
	type data struct {
		Text string
		Attr string
		URL  string
	}
	d := data{
		Text: `<script>alert("xss")</script>`,
		Attr: `" onmouseover="alert(1)`,
		URL:  `javascript:alert(1)`,
	}
	const tmpl = `<p title="{{.Attr}}">{{.Text}}</p><a href="{{.URL}}">link</a>`
	const want = `<p title="&#34; onmouseover=&#34;alert(1)">` +
		`&lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;</p>` +
		`<a href="#ZgotmplZ">link</a>`

	assert.Equal(t, want, execHTML(t, "test", tmpl, d))
}
