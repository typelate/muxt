// Ported from lib/__tests/walk.js.

package csstree

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func notInsideAtrulePrelude(stack []*orderedObject, _ *orderedObject) bool {
	for _, node := range stack {
		if node.values["type"] == "AtrulePrelude" {
			return false
		}
	}
	return true
}

// testWithRules are the fixture files of the atrule, rule and stylesheet
// directories.
func testWithRules(t *testing.T) []astFixtureFile {
	t.Helper()
	var result []astFixtureFile
	for _, file := range loadASTFixtures(t, nil) {
		dir := filepath.Base(filepath.Dir(file.Filename))
		if dir == "atrule" || dir == "rule" || dir == "stylesheet" {
			result = append(result, file)
		}
	}
	return result
}

// expectedWalk walks a fixture AST in Object.values order.
func expectedWalk(ast any, enter, leave bool, checker func(stack []*orderedObject, node *orderedObject) bool) []string {
	if checker == nil {
		checker = func([]*orderedObject, *orderedObject) bool { return true }
	}

	result := []string{}
	var stack []*orderedObject

	var walk func(node *orderedObject)
	walkValue := func(value any) {
		switch value := value.(type) {
		case []any:
			for _, item := range value {
				if object, ok := item.(*orderedObject); ok {
					walk(object)
				}
			}
		case *orderedObject:
			walk(value)
		}
	}
	walk = func(node *orderedObject) {
		nodeType, _ := node.values["type"].(string)

		if enter && checker(stack, node) {
			result = append(result, nodeType)
		}

		stack = append(stack, node)
		for _, key := range node.keys {
			walkValue(node.values[key])
		}
		stack = stack[:len(stack)-1]

		if leave && checker(stack, node) {
			result = append(result, nodeType)
		}
	}

	walkValue(ast)

	return result
}

func createWalkTest(t *testing.T, test astFixtureTest, walker func(ast Node, fn func(node Node)) error, enter, leave bool) {
	if test.Skip {
		t.Skip("skipped by fixture")
	}

	actual := []string{}
	ast, err := Parse(test.Source, test.ParseOptions(false))
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", test.Source, err)
	}

	if err := walker(ast, func(node Node) { actual = append(actual, node.Type()) }); err != nil {
		t.Fatal(err)
	}

	// type arrays should be equal
	if want := expectedWalk(test.OrderedAST, enter, leave, nil); !slices.Equal(actual, want) {
		t.Errorf("Walk(Parse(%q)) = %v, want %v", test.Source, actual, want)
	}
}

func createWalkVisitTest(t *testing.T, test astFixtureTest, visitType string, walker func(ast Node, fn func(node Node)) error) {
	if test.Skip {
		t.Skip("skipped by fixture")
	}

	actual := []string{}
	ast, err := Parse(test.Source, test.ParseOptions(false))
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", test.Source, err)
	}

	if err := walker(ast, func(node Node) { actual = append(actual, node.Type()) }); err != nil {
		t.Fatal(err)
	}

	// type arrays should be equal
	want := []string{}
	for _, nodeType := range expectedWalk(test.OrderedAST, true, false, nil) {
		if nodeType == visitType {
			want = append(want, nodeType)
		}
	}
	slices.Sort(actual)
	slices.Sort(want)
	if !slices.Equal(actual, want) {
		t.Errorf("Walk(Parse(%q), visit %s) = %v, want %v", test.Source, visitType, actual, want)
	}
}

func createWalkDeclarationsTest(t *testing.T, test astFixtureTest, walker func(ast Node, fn func(node Node)) error) {
	if test.Skip {
		t.Skip("skipped by fixture")
	}

	actual := []string{}
	ast, err := Parse(test.Source, test.ParseOptions(false))
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", test.Source, err)
	}

	if err := walker(ast, func(node Node) { actual = append(actual, node.Type()) }); err != nil {
		t.Fatal(err)
	}

	// type arrays should be equal
	want := []string{}
	for _, nodeType := range expectedWalk(test.OrderedAST, false, true, notInsideAtrulePrelude) {
		if nodeType == "Declaration" {
			want = append(want, nodeType)
		}
	}
	slices.Sort(actual)
	slices.Sort(want)
	if !slices.Equal(actual, want) {
		t.Errorf("Walk(Parse(%q), visit Declaration) = %v, want %v", test.Source, actual, want)
	}
}

// walkNodeName is upstream's node.name || node.property || node.mediaType
// for the string fields of those names.
func walkNodeName(node Node) string {
	switch n := node.(type) {
	case *Atrule:
		return n.Name
	case *ClassSelector:
		return n.Name
	case *Combinator:
		return n.Name
	case *Feature:
		return n.Name
	case *Function:
		return n.Name
	case *IDSelector:
		return n.Name
	case *Identifier:
		return n.Name
	case *Layer:
		return n.Name
	case *PseudoClassSelector:
		return n.Name
	case *PseudoElementSelector:
		return n.Name
	case *TypeSelector:
		return n.Name
	case *Declaration:
		return n.Property
	case *MediaQuery:
		return n.MediaType
	}
	return ""
}

// nodeName is upstream's node.type + (node.name || node.property ? ':' + ... : ”).
func walkTestNodeName(node Node) string {
	if _, ok := node.(*MediaQuery); ok {
		return node.Type()
	}
	if name := walkNodeName(node); name != "" {
		return node.Type() + ":" + name
	}
	return node.Type()
}

func mustParse(t *testing.T, source string, options ParseOptions) Node {
	t.Helper()
	ast, err := Parse(source, options)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", source, err)
	}
	return ast
}

func mustWalk(t *testing.T, ast Node, options WalkOptions) {
	t.Helper()
	if err := Walk(ast, options); err != nil {
		t.Fatalf("Walk() error = %v", err)
	}
}

func enterOnly(fn func(node Node)) WalkOptions {
	return WalkOptions{
		Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
			fn(node)
			return WalkContinue
		},
	}
}

func TestWalk(t *testing.T) {
	t.Run("AST traversal", func(t *testing.T) {
		t.Run("base test", func(t *testing.T) {
			ast := mustParse(t, `@import url("test");@media (min-width: 200px) { .foo:nth-child(2n) { color: rgb(100%, 10%, 0%); width: calc(3px + 5%); content: "test" } }`, ParseOptions{})
			visitedTypes := make(map[string]bool)
			expected := []string{
				"AnPlusB",
				"Atrule",
				"AtrulePrelude",
				"Block",
				"ClassSelector",
				"Condition",
				"Declaration",
				"Dimension",
				"Feature",
				"Function",
				"MediaQuery",
				"MediaQueryList",
				"Nth",
				"Operator",
				"Percentage",
				"PseudoClassSelector",
				"Rule",
				"Selector",
				"SelectorList",
				"String",
				"StyleSheet",
				"Url",
				"Value",
			}

			mustWalk(t, ast, enterOnly(func(node Node) { visitedTypes[node.Type()] = true }))

			var actual []string
			for nodeType := range visitedTypes {
				actual = append(actual, nodeType)
			}
			slices.Sort(actual)
			if !slices.Equal(actual, expected) {
				t.Errorf("Walk() visited %v, want %v", actual, expected)
			}
		})

		t.Run("base test #2", func(t *testing.T) {
			ast := mustParse(t, ".a { color: red }", ParseOptions{})
			var log []string

			mustWalk(t, ast, WalkOptions{
				Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
					log = append(log, "enter "+node.Type())
					return WalkContinue
				},
				Leave: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
					log = append(log, "leave "+node.Type())
					return WalkContinue
				},
			})

			expected := []string{
				"enter StyleSheet",
				"enter Rule",
				"enter SelectorList",
				"enter Selector",
				"enter ClassSelector",
				"leave ClassSelector",
				"leave Selector",
				"leave SelectorList",
				"enter Block",
				"enter Declaration",
				"enter Value",
				"enter Identifier",
				"leave Identifier",
				"leave Value",
				"leave Declaration",
				"leave Block",
				"leave Rule",
				"leave StyleSheet",
			}
			if !slices.Equal(log, expected) {
				t.Errorf("Walk() log = %v, want %v", log, expected)
			}
		})

		t.Run("traverse order", func(t *testing.T) {
			ast := mustParse(t, ".a.b { foo: bar; baz: qux } .c {} @media all { .d:not(.e > .f) { aa: bb; cc: dd ee } f { ff: gg } }", ParseOptions{})

			t.Run("natural", func(t *testing.T) {
				var visitedNames []string
				expected := strings.Split("a b foo bar baz qux c media all d not e > f aa bb cc dd ee f ff gg", " ")

				mustWalk(t, ast, enterOnly(func(node Node) {
					if ref := walkNodeName(node); ref != "" {
						visitedNames = append(visitedNames, ref)
					}
				}))

				if !slices.Equal(visitedNames, expected) {
					t.Errorf("Walk() names = %v, want %v", visitedNames, expected)
				}
			})

			t.Run("reverse", func(t *testing.T) {
				var visitedNames []string
				expected := strings.Split("media ff gg f cc ee dd aa bb not f > e d all c baz qux foo bar b a", " ")

				options := enterOnly(func(node Node) {
					if ref := walkNodeName(node); ref != "" {
						visitedNames = append(visitedNames, ref)
					}
				})
				options.Reverse = true
				mustWalk(t, ast, options)

				if !slices.Equal(visitedNames, expected) {
					t.Errorf("Walk(reverse) names = %v, want %v", visitedNames, expected)
				}
			})
		})

		t.Run("break traverse", func(t *testing.T) {
			ast := mustParse(t, ".a.b { foo: bar; } .c {} .a.b { foo: bar; }", ParseOptions{})

			breakTest := func(t *testing.T, reverse bool, expected []string) {
				var actual []string

				mustWalk(t, ast, WalkOptions{
					Reverse: reverse,
					Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
						actual = append(actual, "enter "+walkTestNodeName(node))

						if walkNodeName(node) == "c" {
							return WalkBreak
						}
						return WalkContinue
					},
					Leave: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
						actual = append(actual, "leave "+walkTestNodeName(node))
						return WalkContinue
					},
				})

				if !slices.Equal(actual, expected) {
					t.Errorf("Walk() log =\n%v\nwant\n%v", actual, expected)
				}
			}

			t.Run("natural order", func(t *testing.T) {
				expected := []string{
					"enter StyleSheet",
					"enter Rule",
					"enter SelectorList",
					"enter Selector",
					"enter ClassSelector:a",
					"leave ClassSelector:a",
					"enter ClassSelector:b",
					"leave ClassSelector:b",
					"leave Selector",
					"leave SelectorList",
					"enter Block",
					"enter Declaration:foo",
					"enter Value",
					"enter Identifier:bar",
					"leave Identifier:bar",
					"leave Value",
					"leave Declaration:foo",
					"leave Block",
					"leave Rule",
					"enter Rule",
					"enter SelectorList",
					"enter Selector",
					"enter ClassSelector:c",
				}

				// this.break and walk.break are both WalkBreak
				t.Run("this.break", func(t *testing.T) { breakTest(t, false, expected) })
				t.Run("walk.break", func(t *testing.T) { breakTest(t, false, expected) })
			})

			t.Run("reverse order", func(t *testing.T) {
				expected := []string{
					"enter StyleSheet",
					"enter Rule",
					"enter Block",
					"enter Declaration:foo",
					"enter Value",
					"enter Identifier:bar",
					"leave Identifier:bar",
					"leave Value",
					"leave Declaration:foo",
					"leave Block",
					"enter SelectorList",
					"enter Selector",
					"enter ClassSelector:b",
					"leave ClassSelector:b",
					"enter ClassSelector:a",
					"leave ClassSelector:a",
					"leave Selector",
					"leave SelectorList",
					"leave Rule",
					"enter Rule",
					"enter Block",
					"leave Block",
					"enter SelectorList",
					"enter Selector",
					"enter ClassSelector:c",
				}

				t.Run("this.break", func(t *testing.T) { breakTest(t, true, expected) })
				t.Run("walk.break", func(t *testing.T) { breakTest(t, true, expected) })
			})
		})

		t.Run("skip node traverse", func(t *testing.T) {
			ast := mustParse(t, ".a.b { foo: bar } @media all { selector { foo: bar } } .c.d { foo: bar }", ParseOptions{})

			skipTest := func(t *testing.T, reverse bool, expected []string) {
				var actual []string

				mustWalk(t, ast, WalkOptions{
					Reverse: reverse,
					Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
						if walkNodeName(node) == "media" {
							actual = append(actual, "skip "+walkTestNodeName(node))
							return WalkSkip
						}

						actual = append(actual, "enter "+walkTestNodeName(node))
						return WalkContinue
					},
					Leave: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
						actual = append(actual, "leave "+walkTestNodeName(node))
						return WalkContinue
					},
				})

				if !slices.Equal(actual, expected) {
					t.Errorf("Walk() log =\n%v\nwant\n%v", actual, expected)
				}
			}

			t.Run("natural order", func(t *testing.T) {
				expected := []string{
					"enter StyleSheet",
					"enter Rule",
					"enter SelectorList",
					"enter Selector",
					"enter ClassSelector:a",
					"leave ClassSelector:a",
					"enter ClassSelector:b",
					"leave ClassSelector:b",
					"leave Selector",
					"leave SelectorList",
					"enter Block",
					"enter Declaration:foo",
					"enter Value",
					"enter Identifier:bar",
					"leave Identifier:bar",
					"leave Value",
					"leave Declaration:foo",
					"leave Block",
					"leave Rule",
					"skip Atrule:media",
					"enter Rule",
					"enter SelectorList",
					"enter Selector",
					"enter ClassSelector:c",
					"leave ClassSelector:c",
					"enter ClassSelector:d",
					"leave ClassSelector:d",
					"leave Selector",
					"leave SelectorList",
					"enter Block",
					"enter Declaration:foo",
					"enter Value",
					"enter Identifier:bar",
					"leave Identifier:bar",
					"leave Value",
					"leave Declaration:foo",
					"leave Block",
					"leave Rule",
					"leave StyleSheet",
				}

				// this.skip and walk.skip are both WalkSkip
				t.Run("this.skip", func(t *testing.T) { skipTest(t, false, expected) })
				t.Run("walk.skip", func(t *testing.T) { skipTest(t, false, expected) })
			})

			t.Run("reverse order", func(t *testing.T) {
				expected := []string{
					"enter StyleSheet",
					"enter Rule",
					"enter Block",
					"enter Declaration:foo",
					"enter Value",
					"enter Identifier:bar",
					"leave Identifier:bar",
					"leave Value",
					"leave Declaration:foo",
					"leave Block",
					"enter SelectorList",
					"enter Selector",
					"enter ClassSelector:d",
					"leave ClassSelector:d",
					"enter ClassSelector:c",
					"leave ClassSelector:c",
					"leave Selector",
					"leave SelectorList",
					"leave Rule",
					"skip Atrule:media",
					"enter Rule",
					"enter Block",
					"enter Declaration:foo",
					"enter Value",
					"enter Identifier:bar",
					"leave Identifier:bar",
					"leave Value",
					"leave Declaration:foo",
					"leave Block",
					"enter SelectorList",
					"enter Selector",
					"enter ClassSelector:b",
					"leave ClassSelector:b",
					"enter ClassSelector:a",
					"leave ClassSelector:a",
					"leave Selector",
					"leave SelectorList",
					"leave Rule",
					"leave StyleSheet",
				}

				t.Run("this.skip", func(t *testing.T) { skipTest(t, true, expected) })
				t.Run("walk.skip", func(t *testing.T) { skipTest(t, true, expected) })
			})
		})

		t.Run("bad options", func(t *testing.T) {
			var ast Node = &StyleSheet{Children: new(List[Node])}

			t.Run("should throws when no enter/leave handlers is set", func(t *testing.T) {
				const want = "Neither `enter` nor `leave` walker handler is set or both aren't a function"

				if err := Walk(ast, WalkOptions{}); err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("Walk(ast, {}) error = %v, want %q", err, want)
				}
			})

			t.Run("should throws when visit has wrong value", func(t *testing.T) {
				const want = "Bad value `Foo` for `visit` option"

				if err := Walk(ast, WalkOptions{Visit: "Foo"}); err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("Walk(ast, {visit: 'Foo'}) error = %v, want %q", err, want)
				}
			})
		})

		t.Run("walk(ast, fn)", func(t *testing.T) {
			forEachASTTest(t, false, func(t *testing.T, test astFixtureTest) {
				createWalkTest(t, test, func(ast Node, fn func(node Node)) error {
					return Walk(ast, enterOnly(fn))
				}, true, false)
			})
		})

		t.Run("walk(ast, { leave: fn })", func(t *testing.T) {
			forEachASTTest(t, false, func(t *testing.T, test astFixtureTest) {
				createWalkTest(t, test, func(ast Node, fn func(node Node)) error {
					return Walk(ast, WalkOptions{
						Leave: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
							fn(node)
							return WalkContinue
						},
					})
				}, false, true)
			})
		})

		visitTests := func(t *testing.T, visitType string, reverse bool) {
			for _, file := range testWithRules(t) {
				for _, test := range file.Tests {
					t.Run(test.Name, func(t *testing.T) {
						createWalkVisitTest(t, test, visitType, func(ast Node, fn func(node Node)) error {
							options := enterOnly(fn)
							options.Visit = visitType
							options.Reverse = reverse
							return Walk(ast, options)
						})
					})
				}
			}
		}

		t.Run("walk(ast, { visit: 'Rule' })", func(t *testing.T) {
			visitTests(t, "Rule", false)
		})

		t.Run("walk(ast, { visit: 'Atrule' })", func(t *testing.T) {
			visitTests(t, "Atrule", false)
		})

		t.Run("walk(ast, { visit: 'Rule', reverse: true })", func(t *testing.T) {
			visitTests(t, "Rule", true)
		})

		t.Run("walk(ast, { visit: 'Number' })", func(t *testing.T) {
			visitTests(t, "Number", false)
		})

		t.Run("walk(ast, { visit: 'Declaration' })", func(t *testing.T) {
			for _, file := range testWithRules(t) {
				for _, test := range file.Tests {
					t.Run(test.Name, func(t *testing.T) {
						createWalkDeclarationsTest(t, test, func(ast Node, fn func(node Node)) error {
							options := enterOnly(fn)
							options.Visit = "Declaration"
							return Walk(ast, options)
						})
					})
				}
			}

			t.Run("iterate DeclarationList", func(t *testing.T) {
				ast := mustParse(t, "foo: a; bar: b", ParseOptions{Context: "declarationList"})
				visited := 0

				mustWalk(t, ast, WalkOptions{
					Visit: "Declaration",
					Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
						if node.Type() == "Declaration" {
							visited++
						}
						return WalkContinue
					},
				})

				if visited != 2 {
					t.Errorf("Walk(visit Declaration) visited %d, want 2", visited)
				}
			})
		})
	})
}
