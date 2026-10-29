// Ported from lib/__tests/definition-syntax-match.js and
// lib/__tests/fixture/definition-syntax-match.js.

package csstree

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode"
)

const definitionSyntaxMatchFixtureDir = "testdata/definition-syntax-match"

// definitionSyntaxMatchTest is an entry of a definition-syntax-match
// fixture file.
type definitionSyntaxMatchTest struct {
	Name            string
	Test            string            `json:"test"`
	Syntax          string            `json:"syntax"`
	DefaultSyntaxes bool              `json:"defaultSyntaxes"`
	Types           map[string]string `json:"types"`
	Properties      map[string]string `json:"properties"`
	Valid           []string          `json:"valid"`
	Invalid         []string          `json:"invalid"`
	MatchResult     map[string]any    `json:"matchResult"`
}

// forEachDefinitionSyntaxMatchTest mirrors the fixture's forEachTest: files
// in directory order, tests in key order.
func forEachDefinitionSyntaxMatchTest(t *testing.T, factory func(t *testing.T, testName string, test definitionSyntaxMatchTest)) {
	t.Helper()

	entries, err := os.ReadDir(definitionSyntaxMatchFixtureDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		fn := entry.Name()
		if fn == "index.js" {
			continue
		}

		filename := filepath.Join(definitionSyntaxMatchFixtureDir, fn)
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		locator, err := newJSONLocator(path.Join(definitionSyntaxMatchFixtureDir, fn), data)
		if err != nil {
			t.Fatal(err)
		}

		var tests map[string]definitionSyntaxMatchTest
		if err := json.Unmarshal(data, &tests); err != nil {
			t.Fatalf("%s: %v", filename, err)
		}

		for _, key := range locator.keys {
			test := tests[key]
			test.Name, err = locator.get(key)
			if err != nil {
				t.Fatal(err)
			}

			factory(t, key, test)
		}
	}
}

// testMatchSyntaxes is the syntaxes object of the upstream test: match
// graph handlers by name, built on first use.
type testMatchSyntaxes struct {
	types      map[string]func() (*matchGraphNode, error)
	properties map[string]func() (*matchGraphNode, error)
}

func (s *testMatchSyntaxes) lookupMatch(isType bool, name string) (*matchGraphNode, error) {
	dict := s.properties
	if isType {
		dict = s.types
	}

	handler, ok := dict[name]
	if !ok {
		return nil, nil
	}

	return handler()
}

// createSyntaxMatchHandler returns a lazily built match graph for a
// definition syntax (upstream createSyntaxMatchHanlder).
func createSyntaxMatchHandler(source string) func() (*matchGraphNode, error) {
	return sync.OnceValues(func() (*matchGraphNode, error) {
		return buildMatchGraphFromString(source, nil)
	})
}

// createGenericMatchHandler is createSyntaxMatchHandler for a generic type.
// The graph names the type, as the lexer's do, for the matcher to type
// check math functions.
func createGenericMatchHandler(name string, fn genericFunc) func() (*matchGraphNode, error) {
	match := buildGenericMatchGraphRoot(fn, &SyntaxRef{Kind: "Type", Name: name})
	return func() (*matchGraphNode, error) {
		return match, nil
	}
}

// getDefaultSyntax returns the match graphs of the default lexer's types
// and properties (their match, not matchRef, as upstream).
func getDefaultSyntax() *testMatchSyntaxes {
	syntaxes := &testMatchSyntaxes{
		types:      make(map[string]func() (*matchGraphNode, error)),
		properties: make(map[string]func() (*matchGraphNode, error)),
	}

	for name, def := range DefaultLexer().Types() {
		syntaxes.types[name] = def.match
	}

	for name, def := range DefaultLexer().Properties() {
		syntaxes.properties[name] = def.match
	}

	return syntaxes
}

func processMatchResult(mr any) any {
	if array, ok := mr.([]*MatchNode); ok {
		result := make([]any, len(array))
		for i, item := range array {
			result[i] = processMatchResult(item)
		}
		if len(result) == 1 {
			return result[0]
		}
		return result
	}

	node := mr.(*MatchNode)

	if node.Token != "" {
		return node.Token
	}

	if node.Syntax != nil && node.Match != nil {
		syntax := syntaxName(node.Syntax)
		switch node.Syntax.Type() {
		case "Type":
			syntax = "<" + syntax + ">"
		case "Property":
			syntax = "<'" + syntax + "'>"
		}

		return map[string]any{
			"syntax": syntax,
			"match":  processMatchResult(node.Match),
		}
	}

	return nil
}

func hasNonWhiteSpace(s string) bool {
	return strings.TrimFunc(s, unicode.IsSpace) != ""
}

func createSyntaxTest(t *testing.T, testName string, test definitionSyntaxMatchTest) {
	name := test.Name
	if name == "" {
		name = testName
	}

	t.Run(name, func(t *testing.T) {
		if test.Test == "skip" {
			t.Skip("skipped by fixture")
		}

		syntax := test.Syntax
		if syntax == "" {
			syntax = testName
		}
		matchGraph := createSyntaxMatchHandler(syntax)
		syntaxes := &testMatchSyntaxes{
			types:      make(map[string]func() (*matchGraphNode, error)),
			properties: make(map[string]func() (*matchGraphNode, error)),
		}
		if test.DefaultSyntaxes {
			syntaxes = getDefaultSyntax()
		}

		for _, generic := range createGenericTypes(defaultUnits()) {
			syntaxes.types[generic.name] = createGenericMatchHandler(generic.name, generic.fn)
		}

		for name, source := range test.Types {
			syntaxes.types[name] = createSyntaxMatchHandler(source)
		}

		for name, source := range test.Properties {
			syntaxes.properties[name] = createSyntaxMatchHandler(source)
		}

		valid, invalid := specMathFixtureValues(testName, test.Valid, test.Invalid)
		for _, input := range valid {
			t.Run(`should MATCH to "`+input+`"`, func(t *testing.T) {
				graph, err := matchGraph()
				if err != nil {
					t.Fatal(err)
				}
				m, err := matchAsList(stringToTokens(input), graph, syntaxes)
				if err != nil {
					t.Fatal(err)
				}

				if m.list == nil {
					t.Fatalf("matchAsList(%q) = null, want a match", input)
				}
				var matched []string
				for _, x := range m.list {
					if x.isToken {
						matched = append(matched, x.token)
					}
				}
				var tokens []string
				for _, x := range m.tokens {
					if hasNonWhiteSpace(x.value) {
						tokens = append(tokens, x.value)
					}
				}
				if !slices.Equal(matched, tokens) {
					t.Errorf("matchAsList(%q) tokens = %q, want %q", input, matched, tokens)
				}
			})
		}

		for _, input := range invalid {
			t.Run(`should NOT MATCH to "`+input+`"`, func(t *testing.T) {
				graph, err := matchGraph()
				if err != nil {
					t.Fatal(err)
				}
				m, err := matchAsList(stringToTokens(input), graph, syntaxes)
				if err != nil {
					t.Fatal(err)
				}

				if m.list != nil {
					t.Errorf("matchAsList(%q) matched, want null", input)
				}
			})
		}

		for _, input := range sortedKeys(test.MatchResult) {
			matchResult := test.MatchResult[input]

			t.Run(`match result for "`+input+`"`, func(t *testing.T) {
				graph, err := matchGraph()
				if err != nil {
					t.Fatal(err)
				}
				m, err := matchAsTree(stringToTokens(input), graph, syntaxes)
				if err != nil {
					t.Fatal(err)
				}
				if m.tree == nil {
					t.Fatalf("matchAsTree(%q) = null, want a match", input)
				}

				if got := processMatchResult(m.tree.Match); !reflect.DeepEqual(got, matchResult) {
					t.Errorf("matchAsTree(%q) = %v, want %v", input, got, matchResult)
				}
			})
		}
	})
}

func TestDefinitionSyntaxMatch(t *testing.T) {
	t.Run("syntax matching", func(t *testing.T) {
		forEachDefinitionSyntaxMatchTest(t, createSyntaxTest)

		t.Run("should raise an error on broken type reference", func(t *testing.T) {
			matchTree, err := buildMatchGraphFromString("<foo>", nil)
			if err != nil {
				t.Fatal(err)
			}

			_, err = matchAsList(stringToTokens("foo"), matchTree, &testMatchSyntaxes{})
			pattern := regexp.MustCompile(`Bad syntax reference: <foo>`)
			if err == nil || !pattern.MatchString(err.Error()) {
				t.Errorf("matchAsList() error = %v, want %v", err, pattern)
			}
		})

		t.Run("should raise an error on broken property reference", func(t *testing.T) {
			matchTree, err := buildMatchGraphFromString("<'foo'>", nil)
			if err != nil {
				t.Fatal(err)
			}

			_, err = matchAsList(stringToTokens("foo"), matchTree, &testMatchSyntaxes{})
			pattern := regexp.MustCompile(`Bad syntax reference: <'foo'>`)
			if err == nil || !pattern.MatchString(err.Error()) {
				t.Errorf("matchAsList() error = %v, want %v", err, pattern)
			}
		})
	})
}
