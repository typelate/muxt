package csstree

import (
	"slices"
	"testing"
)

// A reverse walk visits the fields of a node in reverse order (expected
// order is upstream's).
func TestWalkReverseFields(t *testing.T) {
	ast := mustParse(t, ":nth-child(2n of a)[b=c i]{}@media (1px<width<2px) and (aspect-ratio:16/9){}@scope (.d) to (.e){}", ParseOptions{})

	var types []string
	mustWalk(t, ast, WalkOptions{
		Reverse: true,
		Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
			types = append(types, node.Type())
			return WalkContinue
		},
	})

	want := []string{
		"StyleSheet",
		"Atrule", "Block", "AtrulePrelude", "Scope", "SelectorList", "Selector", "ClassSelector", "SelectorList", "Selector", "ClassSelector",
		"Atrule", "Block", "AtrulePrelude", "MediaQueryList", "MediaQuery", "Condition",
		"Feature", "Ratio", "Number", "Number", "Identifier", "FeatureRange", "Dimension", "Identifier", "Dimension",
		"Rule", "Block", "SelectorList", "Selector", "AttributeSelector", "Identifier", "Identifier",
		"PseudoClassSelector", "Nth", "SelectorList", "Selector", "TypeSelector", "AnPlusB",
	}
	if !slices.Equal(types, want) {
		t.Errorf("reverse walk = %q, want %q", types, want)
	}
}

// A walk with Visit honours WalkBreak and WalkSkip (upstream ignores
// walk.break and walk.skip when visit is set; see README).
func TestWalkVisitHonoursBreakAndSkip(t *testing.T) {
	ast := mustParse(t, "a{b:c;d:e}f{g:h}", ParseOptions{})

	t.Run("break in enter", func(t *testing.T) {
		var properties []string
		mustWalk(t, ast, WalkOptions{
			Visit: "Declaration",
			Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
				properties = append(properties, node.(*Declaration).Property)
				return WalkBreak
			},
		})
		if want := []string{"b"}; !slices.Equal(properties, want) {
			t.Errorf("visited %q, want %q", properties, want)
		}
	})

	t.Run("break in leave", func(t *testing.T) {
		var names []string
		mustWalk(t, ast, WalkOptions{
			Visit: "Identifier",
			Leave: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
				names = append(names, node.(*Identifier).Name)
				return WalkBreak
			},
		})
		if want := []string{"c"}; !slices.Equal(names, want) {
			t.Errorf("visited %q, want %q", names, want)
		}
	})

	t.Run("skip", func(t *testing.T) {
		var visited []string
		mustWalk(t, ast, WalkOptions{
			Visit: "Rule",
			Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
				visited = append(visited, "enter")
				return WalkSkip
			},
			Leave: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
				visited = append(visited, "leave")
				return WalkContinue
			},
		})
		if want := []string{"enter", "enter"}; !slices.Equal(visited, want) {
			t.Errorf("visited %q, want %q", visited, want)
		}
	})
}
