package definitionsyntax_test

// Places where the port follows CSS Values and Units Level 4 §2.3
// (multipliers) and §2.4.1 (range notation) instead of upstream csstree
// 3.2.1. Each comment gives what upstream does with the same source.

import (
	"encoding/json"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

func multiplied(term string) string {
	return `{"type":"Group","terms":[` + term + `],"combinator":" ","disallowEmpty":false,"explicit":false}`
}

func TestParseSpecFixes(t *testing.T) {
	const keywordA = `{"type":"Keyword","name":"a"}`
	for _, tt := range []specEdgeTest{
		// {A} means exactly A times, so {0} is exactly zero times; upstream
		// stores max 0, which it also uses for "no upper bound", and turns
		// it into a*. The port encodes an exact zero max as null.
		{source: "a{0}", ast: multiplied(`{"type":"Multiplier","comma":false,"min":0,"max":null,"term":` + keywordA + `}`), generate: "a{0}", compact: "a{0}"},
		{source: "a{0,0}", ast: multiplied(`{"type":"Multiplier","comma":false,"min":0,"max":null,"term":` + keywordA + `}`), generate: "a{0}", compact: "a{0}"},
		{source: "a#{0}", ast: multiplied(`{"type":"Multiplier","comma":true,"min":0,"max":null,"term":` + keywordA + `}`), generate: "a#{0}", compact: "a#{0}"},
		{source: "a{0}?", ast: multiplied(`{"type":"Multiplier","comma":false,"min":0,"max":1,"term":{"type":"Multiplier","comma":false,"min":0,"max":null,"term":` + keywordA + `}}`), generate: "a{0}?", compact: "a{0}?"},
		// unbounded multipliers keep upstream's encoding, max 0
		{source: "a{0,}", ast: multiplied(`{"type":"Multiplier","comma":false,"min":0,"max":0,"term":` + keywordA + `}`), generate: "a*", compact: "a*"},
		{source: "a*", ast: multiplied(`{"type":"Multiplier","comma":false,"min":0,"max":0,"term":` + keywordA + `}`), generate: "a*", compact: "a*"},
		{source: "a#?", ast: multiplied(`{"type":"Multiplier","comma":true,"min":0,"max":0,"term":` + keywordA + `}`), generate: "a#?", compact: "a#?"},
		{source: "a{2,}", ast: multiplied(`{"type":"Multiplier","comma":false,"min":2,"max":0,"term":` + keywordA + `}`), generate: "a{2,}", compact: "a{2,}"},
		{source: "a{2}", ast: multiplied(`{"type":"Multiplier","comma":false,"min":2,"max":2,"term":` + keywordA + `}`), generate: "a{2}", compact: "a{2}"},
		{source: "a{1,1}", ast: multiplied(`{"type":"Multiplier","comma":false,"min":1,"max":1,"term":` + keywordA + `}`), generate: "a", compact: "a"},
		// {A,B} needs A <= B; upstream accepts a{2,1} and a{1,0} (as a+)
		{source: "a{2,1}", err: "Wrong multiplier range", offset: 4},
		{source: "a{1,0}", err: "Wrong multiplier range", offset: 4},
		{source: "a#{3, 2}", err: "Wrong multiplier range", offset: 6},
		// §2.4.1 writes minus infinity, and negative bounds, with U+2212
		// MINUS SIGN; upstream only accepts U+002D HYPHEN-MINUS ("Expect a number")
		{source: "<length [−∞,0]>", ast: multiplied(`{"type":"Type","name":"length","opts":{"type":"Range","min":null,"max":0}}`), generate: "<length [-∞,0]>", compact: "<length [-∞,0]>"},
		{source: "<length [−∞,∞]>", ast: multiplied(`{"type":"Type","name":"length","opts":{"type":"Range","min":null,"max":null}}`), generate: "<length [-∞,∞]>", compact: "<length [-∞,∞]>"},
		{source: "<integer [−10,−1]>", ast: multiplied(`{"type":"Type","name":"integer","opts":{"type":"Range","min":-10,"max":-1}}`), generate: "<integer [-10,-1]>", compact: "<integer [-10,-1]>"},
		{source: "<length [−5px,0]>", ast: multiplied(`{"type":"Type","name":"length","opts":{"type":"Range","min":"-5px","max":0}}`), generate: "<length [-5px,0]>", compact: "<length [-5px,0]>"},
		// minus infinity is only a lower bound, as upstream has it
		{source: "<n [0,−∞]>", err: "Expect a number", offset: 9},
		{source: "<n [0,-∞]>", err: "Expect a number", offset: 7},
	} {
		t.Run(tt.source, func(t *testing.T) {
			runSpecEdgeTest(t, tt)
		})
	}
}

func TestMultiplierUnbounded(t *testing.T) {
	a := &definitionsyntax.Keyword{Name: "a"}
	for _, tt := range []struct {
		node     *definitionsyntax.Multiplier
		generate string
		json     string
	}{
		{
			&definitionsyntax.Multiplier{Min: 0, Max: definitionsyntax.Unbounded, Term: a},
			"a*",
			`{"type":"Multiplier","comma":false,"min":0,"max":0,"term":{"type":"Keyword","name":"a"}}`,
		},
		{
			&definitionsyntax.Multiplier{Min: 3, Max: definitionsyntax.Unbounded, Comma: true, Term: a},
			"a#{3,}",
			`{"type":"Multiplier","comma":true,"min":3,"max":0,"term":{"type":"Keyword","name":"a"}}`,
		},
		{
			&definitionsyntax.Multiplier{Term: a}, // the zero value repeats exactly zero times
			"a{0}",
			`{"type":"Multiplier","comma":false,"min":0,"max":null,"term":{"type":"Keyword","name":"a"}}`,
		},
	} {
		if got := definitionsyntax.Generate(tt.node, definitionsyntax.GenerateOptions{}); got != tt.generate {
			t.Errorf("Generate(%+v) = %q, want %q", *tt.node, got, tt.generate)
		}
		data, err := json.Marshal(tt.node)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != tt.json {
			t.Errorf("json.Marshal(%+v) = %s, want %s", *tt.node, data, tt.json)
		}
	}
}
