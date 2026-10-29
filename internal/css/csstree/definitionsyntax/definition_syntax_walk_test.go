// Ported from lib/__tests/definition-syntax-walk.js.

package definitionsyntax_test

import (
	"regexp"
	"slices"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

type visit struct {
	typeOrAction string
	value        string
}

func TestWalk(t *testing.T) {
	t.Run("pass a single walk function", func(t *testing.T) {
		const source = `a b | c() && [ <d>? || <'e'> || ( f{2,4} ) ]*`
		ast, err := definitionsyntax.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		var visited []visit

		err = definitionsyntax.Walk(ast, definitionsyntax.WalkOptions{
			Enter: func(node definitionsyntax.Node) {
				visited = append(visited, visit{
					typeOrAction: node.Type(),
					value:        definitionsyntax.Generate(node, definitionsyntax.GenerateOptions{}),
				})
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		want := []visit{
			{"Group", `a b | c( ) && [ <d>? || <'e'> || ( f{2,4} ) ]*`},
			{"Group", `a b`}, // implicit group: a b -> [ a b ]
			{"Keyword", `a`},
			{"Keyword", `b`},
			{"Group", `c( ) && [ <d>? || <'e'> || ( f{2,4} ) ]*`},
			{"Group", `c( )`},
			{"Function", `c(`},
			{"Token", `)`},
			{"Multiplier", `[ <d>? || <'e'> || ( f{2,4} ) ]*`},
			{"Group", `[ <d>? || <'e'> || ( f{2,4} ) ]`},
			{"Multiplier", `<d>?`},
			{"Type", `<d>`},
			{"Property", `<'e'>`},
			{"Group", `( f{2,4} )`},
			{"Token", `(`},
			{"Multiplier", `f{2,4}`},
			{"Keyword", `f`},
			{"Token", `)`},
		}
		if !slices.Equal(visited, want) {
			t.Errorf("Walk(Parse(%q)) visited\n%q\nwant\n%q", source, visited, want)
		}
	})

	t.Run("pass a pair of walk functions", func(t *testing.T) {
		const source = `a b? | c() && [ <d> ]+`
		ast, err := definitionsyntax.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		var visited []visit

		err = definitionsyntax.Walk(ast, definitionsyntax.WalkOptions{
			Enter: func(node definitionsyntax.Node) {
				visited = append(visited, visit{
					typeOrAction: "enter",
					value:        definitionsyntax.Generate(node, definitionsyntax.GenerateOptions{}),
				})
			},
			Leave: func(node definitionsyntax.Node) {
				visited = append(visited, visit{
					typeOrAction: "leave",
					value:        definitionsyntax.Generate(node, definitionsyntax.GenerateOptions{}),
				})
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		want := []visit{
			{"enter", `a b? | c( ) && [ <d> ]+`},
			{"enter", `a b?`},
			{"enter", `a`},
			{"leave", `a`},
			{"enter", `b?`},
			{"enter", `b`},
			{"leave", `b`},
			{"leave", `b?`},
			{"leave", `a b?`},
			{"enter", `c( ) && [ <d> ]+`},
			{"enter", `c( )`},
			{"enter", `c(`},
			{"leave", `c(`},
			{"enter", `)`},
			{"leave", `)`},
			{"leave", `c( )`},
			{"enter", `[ <d> ]+`},
			{"enter", `[ <d> ]`},
			{"enter", `<d>`},
			{"leave", `<d>`},
			{"leave", `[ <d> ]`},
			{"leave", `[ <d> ]+`},
			{"leave", `c( ) && [ <d> ]+`},
			{"leave", `a b? | c( ) && [ <d> ]+`},
		}
		if !slices.Equal(visited, want) {
			t.Errorf("Walk(Parse(%q)) visited\n%q\nwant\n%q", source, visited, want)
		}
	})

	// Upstream passes nothing in the first test and {} in the second; both are
	// the zero WalkOptions in Go.
	for _, name := range []string{
		"should throw an exception when nothing passed as walker handler",
		"should throw an exception when passed object has no enter or leave methods",
	} {
		t.Run(name, func(t *testing.T) {
			ast, err := definitionsyntax.Parse(`a | b`)
			if err != nil {
				t.Fatal(err)
			}

			err = definitionsyntax.Walk(ast, definitionsyntax.WalkOptions{})
			pattern := regexp.MustCompile("Neither `enter` nor `leave` walker handler is set or both aren't a function")
			if err == nil || !pattern.MatchString(err.Error()) {
				t.Errorf("Walk(Parse(%q), {}) = %v, want error matching %q", "a | b", err, pattern)
			}
		})
	}
}
