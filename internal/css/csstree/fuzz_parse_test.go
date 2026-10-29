package csstree

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

var fuzzContexts = []string{"stylesheet", "atrule", "atrulePrelude", "mediaQueryList", "mediaQuery", "condition", "rule", "selectorList", "selector", "block", "declarationList", "declaration", "value"}

func FuzzParse(f *testing.F) {
	paths, err := filepath.Glob("testdata/ast/*/*.json")
	if err != nil {
		f.Fatal(err)
	}
	more, err := filepath.Glob("testdata/ast/*/*/*.json")
	if err != nil {
		f.Fatal(err)
	}
	for _, path := range append(paths, more...) {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		var tests map[string]json.RawMessage
		if err := json.Unmarshal(data, &tests); err != nil {
			f.Fatal(err)
		}
		for _, raw := range tests {
			var one struct{ Source string }
			if json.Unmarshal(raw, &one) == nil && one.Source != "" {
				f.Add(one.Source, uint8(0), false)
				continue
			}
			var many []struct{ Source string }
			if json.Unmarshal(raw, &many) == nil {
				for _, t := range many {
					f.Add(t.Source, uint8(0), false)
				}
			}
		}
	}
	f.Fuzz(func(t *testing.T, source string, context uint8, positions bool) {
		opts := ParseOptions{Context: fuzzContexts[int(context)%len(fuzzContexts)], Positions: positions}
		ast, err := Parse(source, opts)
		if err != nil {
			return
		}
		if _, err := Generate(ast, GenerateOptions{}); err != nil {
			t.Fatalf("Generate after Parse(%q, %+v) = %v", source, opts, err)
		}
	})
}
