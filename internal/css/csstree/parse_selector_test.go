// Ported from lib/__tests/parse-selector.js.

package csstree

import "testing"

func TestParseSelector(t *testing.T) {
	t.Run("parse-selector", func(t *testing.T) {
		forEachSelectorASTTest(t, false, func(t *testing.T, test astFixtureTest) {
			if test.Skip {
				t.Skip("skipped by fixture")
			}

			ast, err := ParseSelector(test.Source, test.ParseOptions(false))
			if err != nil {
				t.Fatalf("ParseSelector(%q) error = %v", test.Source, err)
			}

			// AST should be equal
			assertASTEqualNoLoc(t, test.Source, ast, test.AST)
		})
	})
}
