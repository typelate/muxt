package csstree

import (
	"sync"
	"testing"
)

// FuzzMatchProperty matches arbitrary values, as text and as parsed ASTs,
// against properties of the default lexer: it must never panic, and a
// mismatch must come with an error.
func FuzzMatchProperty(f *testing.F) {
	for _, seed := range []struct{ property, value string }{
		{"color", "red"},
		{"margin", "1px auto 0 -2em"},
		{"background", "url(x.png) no-repeat 0 0 / cover, linear-gradient(red, blue)"},
		{"font", "bold 12px/1.5 Arial, sans-serif"},
		{"grid-template-columns", "[a] repeat(3, minmax(10px, 1fr)) [b]"},
		{"unicode-range", "U+0025-00FF, u+4??"},
		{"width", "calc(100% - 2 * var(--x))"},
		{"transition", "opacity 1s ease-in 2s, transform .5s"},
		{"color", "rgb(1 2 3 / 50%)"},
		{"line-height", "0"},
		{"--custom", "anything"},
		{"unknown", "1px"},
	} {
		f.Add(seed.property, seed.value)
	}

	f.Fuzz(func(t *testing.T, property, value string) {
		lexer := DefaultLexer()

		checkResult := func(call string, result *MatchResult) {
			if result.Matched == nil && result.Error == nil {
				t.Errorf("%s = no match and no error", call)
			}
			if result.Matched != nil && result.Error != nil {
				t.Errorf("%s = a match and error %v", call, result.Error)
			}
		}

		checkResult("MatchPropertyString", lexer.MatchPropertyString(property, value))

		ast, err := Parse(value, ParseOptions{Context: "value", Positions: true})
		if err != nil {
			return
		}
		checkResult("MatchProperty", lexer.MatchProperty(property, ast))
		if _, err := lexer.FindValueFragments(property, ast, "Type", "color"); err != nil {
			t.Errorf("FindValueFragments() error = %v", err)
		}
	})
}

// TestLexerConcurrentMatch checks that the shared default lexer can match
// from several goroutines while its syntaxes are parsed on first use (Go
// only; run with -race).
func TestLexerConcurrentMatch(t *testing.T) {
	lexer, err := CreateLexer(&Config{
		Generic: new(true),
		Types: map[string]string{
			"foo": "<bar>#",
			"bar": "[ 1 | 2 | 3 ] | <length>",
		},
		Properties: map[string]string{
			"test":  "<foo> && <'other'>?",
			"other": "a | b",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for _, value := range []string{"1, 2px", "1 a", "b 3", "x"} {
				lexer.MatchPropertyString("test", value)
				DefaultLexer().MatchPropertyString("background", value)
			}
		})
	}
	wg.Wait()
}
