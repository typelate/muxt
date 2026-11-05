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
		{text: `</P `, want: tagScan{part: tagStart, element: "p"}},
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
