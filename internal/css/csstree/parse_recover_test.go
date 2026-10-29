package csstree

import (
	"errors"
	"runtime"
	"testing"
)

// Upstream's parseWithFallback catches any exception, not only syntax errors,
// and whatever else the parse throws reaches the caller. A bug in a node
// parser (here a nil dereference in an extension) must therefore fall back to
// a Raw node or come back from Parse as an error, never crash the caller.
func TestParseRecoversFromPanics(t *testing.T) {
	extended, err := Fork(ExtendFunc(func(syntaxConfig *Config) *Config {
		defaultGetNode := syntaxConfig.Scope["Value"].GetNode

		syntaxConfig.Scope["Value"].GetNode = func(p *Parser, context *SequenceContext) Node {
			if p.IsDelim(dollarSign, 0) {
				var raw *Raw
				return &Raw{Value: raw.Value} // nil dereference
			}

			return defaultGetNode(p, context)
		}

		return syntaxConfig
	}))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("parseWithFallback falls back to Raw", func(t *testing.T) {
		var reported error
		ast, err := extended.Parse("color: $a", ParseOptions{
			Context: "declaration",
			OnParseError: func(err error, _ Node) error {
				reported = err
				return nil
			},
		})
		if err != nil {
			t.Fatalf("Parse(%q) error = %v, want nil", "color: $a", err)
		}
		if raw, ok := ast.(*Declaration).Value.(*Raw); !ok || raw.Value != "$a" {
			t.Errorf("Parse(%q).Value = %#v, want Raw $a", "color: $a", ast.(*Declaration).Value)
		}
		var runtimeError runtime.Error
		if !errors.As(reported, &runtimeError) {
			t.Errorf("OnParseError error = %v, want a runtime.Error", reported)
		}
	})

	t.Run("a panic outside parseWithFallback is returned", func(t *testing.T) {
		_, err := extended.Parse("$a", ParseOptions{Context: "value"})
		var runtimeError runtime.Error
		if !errors.As(err, &runtimeError) {
			t.Errorf("Parse(%q) error = %v, want a runtime.Error", "$a", err)
		}
	})
}
