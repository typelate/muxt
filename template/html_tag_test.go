package template

import "testing"

func TestTagScan(t *testing.T) {
	for _, tt := range []struct {
		text string
		want tagScan
	}{
		{text: `text`, want: tagScan{}},
		{text: `a < b`, want: tagScan{}},
		{text: `<!-- c -->`, want: tagScan{}},
		{text: `<p>`, want: tagScan{}},
		{text: `<p id=i>`, want: tagScan{}},
		{text: `<br/>`, want: tagScan{}},
		{text: `</p>`, want: tagScan{}},
		{text: `<p`, want: tagScan{part: tagStart, element: "p"}},
		{text: `<my-el `, want: tagScan{part: tagStart, element: "my-el"}},
		{text: `</P `, want: tagScan{part: tagStart, element: "p", end: true}},
		{text: `</p id="a`, want: tagScan{part: tagValue, element: "p", end: true, attr: "id", quote: '"'}},
		{text: `</style>`, want: tagScan{}},
		{text: `</script>`, want: tagScan{}},
		{text: `<p id=i `, want: tagScan{part: tagStart, element: "p"}},
		{text: `<p title="a b" `, want: tagScan{part: tagStart, element: "p"}},
		{text: `<p data-`, want: tagScan{part: tagAttrName, element: "p", attr: "data-"}},
		{text: `<a HREF`, want: tagScan{part: tagAttrName, element: "a", attr: "href"}},
		{text: `<a href `, want: tagScan{part: tagAfterName, element: "a", attr: "href"}},
		{text: `<a href=`, want: tagScan{part: tagBeforeValue, element: "a", attr: "href"}},
		{text: `<a href = `, want: tagScan{part: tagBeforeValue, element: "a", attr: "href"}},
		{text: `<a href=/x`, want: tagScan{part: tagValue, element: "a", attr: "href"}},
		{text: `<a href="/x`, want: tagScan{part: tagValue, element: "a", attr: "href", quote: '"'}},
		{text: `<a href='/x > y`, want: tagScan{part: tagValue, element: "a", attr: "href", quote: '\''}},
		{text: `<p "`, want: tagScan{part: tagUnknown}},
		{text: `<p =v`, want: tagScan{part: tagUnknown}},
		{text: `<p id=a"b`, want: tagScan{part: tagUnknown}},
		{text: `<script>`, want: tagScan{part: tagUnknown}},
		{text: `a <`, want: tagScan{}},
		{text: `a </`, want: tagScan{}},
	} {
		if got := (tagScan{}).text(tt.text); got != tt.want {
			t.Errorf("tagScan{}.text(%q) = %+v, want %+v", tt.text, got, tt.want)
		}
	}
}

func TestTagScanAction(t *testing.T) {
	for _, tt := range []struct {
		text string // the text before the action
		want tagScan
	}{
		{text: `<p `, want: tagScan{part: tagAttrName, element: "p"}},
		{text: `<p id `, want: tagScan{part: tagAttrName, element: "p"}},
		{text: `<p data-`, want: tagScan{part: tagAttrName, element: "p", attr: "data-"}},
		{text: `<a href=`, want: tagScan{part: tagValue, element: "a", attr: "href"}},
		{text: `<a href="`, want: tagScan{part: tagValue, element: "a", attr: "href", quote: '"'}},
	} {
		at, after := (tagScan{}).text(tt.text).action()
		if at != tt.want || after != tt.want {
			t.Errorf("tagScan{}.text(%q).action() = %+v, %+v, want %+v twice", tt.text, at, after, tt.want)
		}
	}
}

func TestHTMLNameFilter(t *testing.T) {
	for _, tt := range []struct {
		name  string
		trust Trust
		want  string
	}{
		{name: "title", want: "title"},
		{name: "Hidden", want: "hidden"},
		{name: "h1", want: "h1"},
		{name: "", want: "ZgotmplZ"},
		{name: "a b", want: "ZgotmplZ"},
		{name: "data-x", want: "ZgotmplZ"},
		{name: "onclick", want: "ZgotmplZ"},
		{name: "onanything", want: "ZgotmplZ"},
		{name: "href", want: "ZgotmplZ"},
		{name: "SRC", want: "ZgotmplZ"},
		{name: "style", want: "ZgotmplZ"},
		{name: "srcset", want: "ZgotmplZ"},
		{name: "value", want: "ZgotmplZ"},
		{name: "tweeturl", want: "ZgotmplZ"},
		{name: `title="x"`, want: "ZgotmplZ"},
		{name: `title="x"`, trust: TrustHTMLAttr, want: `title="x"`},
	} {
		if got := htmlNameFilter(tt.name, tt.trust); got != tt.want {
			t.Errorf("htmlNameFilter(%q, %v) = %q, want %q", tt.name, tt.trust, got, tt.want)
		}
	}
}

func TestTagScanWrite(t *testing.T) {
	for _, tt := range []struct {
		text, want string
	}{
		{text: `a b`, want: `a b`},
		{text: `<p id=i>x</p>`, want: `<p id=i>x</p>`},
		{text: `<p title="a<b">`, want: `<p title="a<b">`},
		{text: `<!-- a < b -->`, want: `<!-- a < b -->`},
		{text: `<!DOCTYPE html>`, want: `<!DOCTYPE html>`},
		{text: `a < b`, want: `a &lt; b`},
		{text: `a <`, want: `a &lt;`},
		{text: `a </`, want: `a &lt;/`},
		{text: `1<2 <b>`, want: `1&lt;2 <b>`},
		{text: `<<p>`, want: `&lt;<p>`},
		{text: `<?x`, want: `&lt;?x`},
	} {
		if got, _ := (tagScan{}).write(tt.text); got != tt.want {
			t.Errorf("tagScan{}.write(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestValueAttr(t *testing.T) {
	for name, want := range map[string]string{
		"":              "",
		"title":         "title",
		"on":            "onclick",
		"onanything":    "onclick",
		"data-on":       "onclick",
		"href":          "href",
		"xlink:href":    "href",
		"xmlns:x":       "href",
		"data-tweeturl": "href",
		"srcset":        "srcset",
		"style":         "style",
		"value":         "value",
	} {
		if got := valueAttr(name); got != want {
			t.Errorf("valueAttr(%q) = %q, want %q", name, got, want)
		}
	}
}
