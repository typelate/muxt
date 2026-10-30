package rendering

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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

		// Review fixes (2026-10-30).
		{a: `<p>a <span style="display:var(--d)"></span> b</p>`, b: `<p>a <span style="display:var(--d)"></span>b</p>`, equivalent: false},
		{a: `<p>a <span style="display:block;float:left">f</span> b</p>`, b: `<p>a<span style="display:block;float:left">f</span>b</p>`, equivalent: false},
		{a: `<p>a <span style="display:block;position:absolute">f</span> b</p>`, b: `<p>a<span style="display:block;position:absolute">f</span>b</p>`, equivalent: false},
		{a: `<style>.f{float:left}</style><span>a <div class=f>f</div> b</span>`, b: `<style>.f{float:left}</style><span>a<div class=f>f</div>b</span>`, equivalent: false},
		{a: `<!DOCTYPE html><html><body>a</body></html>`, b: `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd"><html><body>a</body></html>`, equivalent: false},
		{a: `<p>a&#13;b</p>`, b: `<p>a b</p>`, equivalent: false},
		{a: "<p>中 <span>\n文</span></p>", b: `<p>中 <span>文</span></p>`, equivalent: false},
		{a: `<xmp>a  b</xmp>`, b: `<xmp>a b</xmp>`, equivalent: false},
		{a: `<noscript>a  b</noscript>`, b: `<noscript>a b</noscript>`, equivalent: false},
		{a: "<!DOCTYPE html>\n<html>\n<head>\n  <title>t</title>\n</head>\n<body>\n  <p>a</p>\n</body>\n</html>\n", b: `<!DOCTYPE html><html><head><title>t</title></head><body><p>a</p></body></html>`, equivalent: true},
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

// TestWPTOracle checks the oracle against the web-platform-tests
// css/css-text/white-space fixtures vendored in testdata/wpt. Each case names
// the WPT file and the element IDs or rows its inputs came from.
func TestWPTOracle(t *testing.T) {
	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "wpt", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	diff := func(t *testing.T, a, b string, equivalent bool) {
		t.Helper()
		got, err := Diff(a, b, Body, nil)
		if err != nil {
			t.Fatalf("Diff(%q, %q) error: %v", a, b, err)
		}
		if (got == "") != equivalent {
			t.Errorf("Diff(%q, %q) = %q, want equivalent=%v", a, b, got, equivalent)
		}
	}
	// spans returns the decoded <span> contents of <div id=...> elements by ID.
	spans := func(t *testing.T, name string) map[string]string {
		t.Helper()
		out := make(map[string]string)
		for _, m := range regexp.MustCompile(`(?s)<div id=['"]([^'"]+)['"] class="(?:test|ref)"><span>(.*?)</span></div>`).FindAllStringSubmatch(read(t, name), -1) {
			out[m[1]] = html.UnescapeString(m[2])
		}
		return out
	}
	rows := func(t *testing.T, name string, re string) []string {
		t.Helper()
		var out []string
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(read(t, name), -1) {
			out = append(out, m[1])
		}
		if len(out) == 0 {
			t.Fatalf("no rows matching %s in %s", re, name)
		}
		return out
	}
	spaceRuns := regexp.MustCompile(` +`)

	t.Run("white-space-collapse-000", func(t *testing.T) {
		s := spans(t, "white-space-collapse-000.html")
		for _, id := range []string{"", "W", "F", "H"} {
			test, ref := s["test"+id], s["ref"+id]
			if test == "" || ref == "" {
				t.Fatalf("missing test%[1]s or ref%[1]s", id)
			}
			diff(t, "<span>"+test+"</span>", "<span>"+ref+"</span>", true)
		}
	})

	t.Run("white-space-collapse-001", func(t *testing.T) {
		s := spans(t, "white-space-collapse-001.html")
		for _, id := range []string{"NB", "EN", "EM", "TS", "ZW", "NNB", "IS", "IS2"} {
			test, ref := s["test"+id], s["ref"+id]
			if test == "" || ref == "" {
				t.Fatalf("missing test%[1]s or ref%[1]s", id)
			}
			// Non-ASCII spaces are content: the test collapses to its ref.
			diff(t, "<span>"+test+"</span>", "<span>"+ref+"</span>", true)
			// Replacing the literal with U+0020 changes the rendering.
			plain := strings.NewReplacer(" ", " ", " ", " ", " ", " ", " ", " ", "​", " ", " ", " ", "　", " ").Replace(test)
			if plain == test {
				t.Fatalf("test%s has no non-ASCII space to replace", id)
			}
			diff(t, "<span>"+plain+"</span>", "<span>"+ref+"</span>", false)
		}
	})

	t.Run("seg-break-transformation-000.tentative", func(t *testing.T) {
		// CSS Text 4 converts the segment break to U+0020 for these inputs;
		// the oracle keeps U+000A distinct from U+0020, so they are not equal.
		s := spans(t, "seg-break-transformation-000.tentative.html")
		for _, id := range []string{"test1", "test2", "test3", "test4", "test5", "test6"} {
			if s[id] == "" {
				t.Fatalf("missing %s", id)
			}
			diff(t, "<span>"+s[id]+"</span>", "<span>"+s["ref"]+"</span>", false)
		}
	})

	for _, name := range []string{"line-edge-white-space-collapse-001.html", "line-edge-white-space-collapse-002.html"} {
		t.Run(name, func(t *testing.T) {
			for _, row := range rows(t, name, `(?m)^<div>(.*)</div>$`) {
				diff(t, "<div>"+row+"</div>", "<div>"+spaceRuns.ReplaceAllString(row, "")+"</div>", true)
			}
		})
	}

	t.Run("trailing-space-before-br-001", func(t *testing.T) {
		got := rows(t, "trailing-space-before-br-001.html", `(?m)^  <div>(.*)</div>$`)
		if len(got) != 8 {
			t.Fatalf("got %d rows, want 8", len(got))
		}
		// Rows 1-4 are text only. Rows 5-8 end in a <br>, which the oracle
		// treats as content, so they are compared with row 5 instead.
		for _, i := range []int{1, 2, 3} {
			diff(t, "<div>"+got[i]+"</div>", "<div>"+got[0]+"</div>", true)
		}
		for _, i := range []int{5, 6, 7} {
			diff(t, "<div>"+got[i]+"</div>", "<div>"+got[4]+"</div>", true)
		}
		diff(t, "<div>"+got[4]+"</div>", "<div>"+got[0]+"</div>", false)
	})

	t.Run("pre-line-with-space-and-newline", func(t *testing.T) {
		src := read(t, "pre-line-with-space-and-newline.html")
		preLine := `<i style="white-space:pre-line">`
		matches := regexp.MustCompile(`(?m)^  <span>(.*)</span>$`).FindAllStringSubmatch(src, -1)
		if len(matches) != 4 {
			t.Fatalf("got %d rows, want 4", len(matches))
		}
		for _, m := range matches {
			row := strings.ReplaceAll(m[1], "<i>", preLine)
			spaceless := strings.NewReplacer(` &#10;`, `&#10;`, `<i> </i>`, `<i></i>`).Replace(m[1])
			spaceless = strings.ReplaceAll(spaceless, "<i>", preLine)
			noNewline := strings.ReplaceAll(row, "&#10;", "")
			diff(t, "<span>"+row+"</span>", "<span>"+spaceless+"</span>", true)
			diff(t, "<span>"+row+"</span>", "<span>"+noNewline+"</span>", false)
		}
	})
}
