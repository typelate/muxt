// Not ported: Go-only checks of behavior other packages rely on. The expected
// JSON is upstream's JSON.stringify(parse(source)) and generate output.

package definitionsyntax_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

func TestParseJSON(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{
			source: `<angle [-90deg,90deg]>#{1,2} | <'width'>+#`,
			want:   `{"type":"Group","terms":[{"type":"Multiplier","comma":true,"min":1,"max":2,"term":{"type":"Type","name":"angle","opts":{"type":"Range","min":"-90deg","max":"90deg"}}},{"type":"Multiplier","comma":true,"min":1,"max":0,"term":{"type":"Multiplier","comma":false,"min":1,"max":0,"term":{"type":"Property","name":"width"}}}],"combinator":"|","disallowEmpty":false,"explicit":false}`,
		},
		{
			source: `<boolean-expr[ <x> && @y ]>? , f( a ) '/' [ <n [-∞,0]> ]!`,
			want:   `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":0,"max":1,"term":{"type":"Boolean","term":{"type":"Group","terms":[{"type":"Type","name":"x","opts":null},{"type":"AtKeyword","name":"y"}],"combinator":"&&","disallowEmpty":false,"explicit":false}}},{"type":"Comma"},{"type":"Function","name":"f"},{"type":"Keyword","name":"a"},{"type":"Token","value":")"},{"type":"String","value":"'/'"},{"type":"Group","terms":[{"type":"Type","name":"n","opts":{"type":"Range","min":null,"max":0}}],"combinator":" ","disallowEmpty":true,"explicit":true}],"combinator":" ","disallowEmpty":false,"explicit":false}`,
		},
	} {
		t.Run(test.source, func(t *testing.T) {
			ast, err := definitionsyntax.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(ast)
			if err != nil {
				t.Fatal(err)
			}

			var got, want any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(test.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Parse(%q) = %s, want %s", test.source, data, test.want)
			}
		})
	}
}

func TestGenerateCompact(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: `a | b`, want: `a|b`},
		{source: `[ a b ] && c`, want: `[a b]&&c`},
	} {
		ast, err := definitionsyntax.Parse(test.source)
		if err != nil {
			t.Fatal(err)
		}
		if got := definitionsyntax.Generate(ast, definitionsyntax.GenerateOptions{Compact: true}); got != test.want {
			t.Errorf("Generate(Parse(%q), {compact}) = %q, want %q", test.source, got, test.want)
		}
	}
}

func TestNilNode(t *testing.T) {
	if got := definitionsyntax.Generate(nil, definitionsyntax.GenerateOptions{}); got != "" {
		t.Errorf("Generate(nil) = %q, want %q", got, "")
	}
	err := definitionsyntax.Walk(&definitionsyntax.Multiplier{}, definitionsyntax.WalkOptions{Enter: func(definitionsyntax.Node) {}})
	if err == nil || err.Error() != "Unknown type: <nil>" {
		t.Errorf("Walk(&Multiplier{}) = %v, want Unknown type: <nil>", err)
	}
}
