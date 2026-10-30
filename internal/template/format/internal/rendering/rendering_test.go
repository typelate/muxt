package rendering

import (
	"slices"
	"testing"
)

func TestDiff(t *testing.T) {
	for _, test := range []struct {
		a, b       string
		equivalent bool
	}{
		{a: `<p>a  b</p>`, b: `<p>a b</p>`, equivalent: true},
		{a: "<p>a\tb</p>", b: `<p>a b</p>`, equivalent: true},
		{a: "<p>a\nb</p>", b: `<p>a b</p>`, equivalent: false},
		{a: "<p>a\n\n b</p>", b: "<p>a\nb</p>", equivalent: true},
		{a: `<p> a </p>`, b: `<p>a</p>`, equivalent: true},
		{a: `<p><span> a</span></p>`, b: `<p><span>a</span></p>`, equivalent: true},
		{a: `<p>a<span> b</span></p>`, b: `<p>a <span>b</span></p>`, equivalent: false},
		{a: `<p>a <span> b</span></p>`, b: `<p>a <span>b</span></p>`, equivalent: true},
		{a: "<p>a b</p>", b: `<p>a b</p>`, equivalent: false},
		{a: "<p>\fa</p>", b: `<p>a</p>`, equivalent: false},
		{a: `<pre> a</pre>`, b: `<pre>a</pre>`, equivalent: false},
		{a: `<div>a</div> <div>b</div>`, b: `<div>a</div><div>b</div>`, equivalent: true},
		{a: `<span>a</span> <span>b</span>`, b: `<span>a</span><span>b</span>`, equivalent: false},
		{a: `<table><tr><td>a</td> <td>b</td></tr></table>`, b: `<table><tr><td>a</td><td>b</td></tr></table>`, equivalent: true},
		{a: `a<br> b`, b: `a<br>b`, equivalent: true},
		{a: `<div style="display:flex"> <span>a</span> </div>`, b: `<div style="display:flex"><span>a</span></div>`, equivalent: true},
		{a: "<p style=\"white-space:pre-line\">a \n b</p>", b: "<p style=\"white-space:pre-line\">a\nb</p>", equivalent: true},
		{a: "<p style=\"white-space:pre-line\">a\nb</p>", b: "<p style=\"white-space:pre-line\">a\n\nb</p>", equivalent: false},
		{a: `<p>a<!--x-->b</p>`, b: `<p>ab</p>`, equivalent: false},
		{a: `<script> x  </script>`, b: `<script> x </script>`, equivalent: false},
		{a: `<style>.c{white-space:pre}</style><p class="c"> a</p>`, b: `<style>.c{white-space:pre}</style><p class="c">a</p>`, equivalent: false},
		{a: `<p>a<div>b</div>c</p>`, b: `<p>a</p><div>b</div>c<p></p>`, equivalent: true},
	} {
		t.Run(test.a, func(t *testing.T) {
			got, err := Diff(test.a, test.b, Body, nil)
			if err != nil {
				t.Fatalf("Diff(%q, %q) error: %v", test.a, test.b, err)
			}
			if (got == "") != test.equivalent {
				t.Errorf("Diff(%q, %q) = %q, want equivalent=%v", test.a, test.b, got, test.equivalent)
			}
		})
	}
}

func TestTokensTableBody(t *testing.T) {
	got, err := Tokens(`<tr><td>a</td></tr>`, TableBody, nil)
	if err != nil {
		t.Fatalf("Tokens error: %v", err)
	}
	want := []Token{
		{Kind: Open, Data: "tr"},
		{Kind: Open, Data: "td"},
		{Kind: Text, Data: "a"},
		{Kind: Close, Data: "td"},
		{Kind: Close, Data: "tr"},
	}
	if !slices.EqualFunc(got, want, tokenEqual) {
		t.Errorf("Tokens(%q, TableBody, nil) = %v, want %v", `<tr><td>a</td></tr>`, got, want)
	}
}

func tokenEqual(a, b Token) bool {
	return a.Kind == b.Kind && a.Data == b.Data && slices.Equal(a.Attr, b.Attr)
}
