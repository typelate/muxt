// Ported from lib/__tests/parse.js.

package csstree

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

type genericTypeFixture struct {
	Valid   []string `json:"valid"`
	Invalid []string `json:"invalid"`
}

func loadGenericTypesFixture(t *testing.T) map[string]genericTypeFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/definition-syntax-match/generic.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]genericTypeFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func createParseErrorTest(t *testing.T, name string, test astFixtureTest, options ParseOptions) {
	t.Run(name+" "+jsonString(test.Source), func(t *testing.T) {
		if test.Skip {
			t.Skip("skipped by fixture")
		}

		_, err := Parse(test.Source, options)
		if err == nil {
			t.Fatalf("Parse(%q) = nil error, want %q", test.Source, test.Error)
		}

		var syntaxError *SyntaxError
		if !errors.As(err, &syntaxError) {
			t.Fatalf("Parse(%q) error = %T %v, want a *SyntaxError", test.Source, err, err)
		}

		if syntaxError.Message != test.Error {
			t.Errorf("Parse(%q) error message = %q, want %q", test.Source, syntaxError.Message, test.Error)
		}

		got := astFixturePosition{
			Offset: syntaxError.Offset,
			Line:   syntaxError.Line,
			Column: syntaxError.Column,
		}
		if got != test.Position {
			t.Errorf("Parse(%q) error position = %+v, want %+v", test.Source, got, test.Position)
		}
	})
}

func jsonString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestParse(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		forEachASTTest(t, false, func(t *testing.T, test astFixtureTest) {
			if test.Skip {
				t.Skip("skipped by fixture")
			}

			ast, err := Parse(test.Source, test.ParseOptions(false))
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", test.Source, err)
			}

			// AST should be equal
			assertASTEqualNoLoc(t, test.Source, ast, test.AST)
		})

		genericTypesFixture := loadGenericTypesFixture(t)

		t.Run("AnPlusB", func(t *testing.T) {
			fixture := genericTypesFixture["<an-plus-b>"]
			aPattern := regexp.MustCompile(`(?i)^([+-]?)(\d+)?n`)
			bPattern := regexp.MustCompile(`([+-]?)\s*(\d+)$`)

			for _, value := range fixture.Valid {
				t.Run(value, func(t *testing.T) {
					ast, err := Parse(":nth-child("+value+")", ParseOptions{Context: "selector"})
					if err != nil {
						t.Fatalf("Parse(%q) error = %v", value, err)
					}
					actual := ast.(*Selector).Children.First().(*PseudoClassSelector).Children.First().(*Nth).Nth
					a := aPattern.FindStringSubmatch(value)
					b := bPattern.FindStringSubmatch(value)
					expected := map[string]any{
						"type": "AnPlusB",
						"a":    nil,
						"b":    nil,
					}
					if a != nil {
						sign := ""
						if a[1] == "-" {
							sign = "-"
						}
						digits := a[2]
						if digits == "" {
							digits = "1"
						}
						expected["a"] = sign + digits
					}
					if b != nil {
						sign := ""
						if b[1] == "-" {
							sign = "-"
						}
						expected["b"] = sign + b[2]
					}

					// AST should be equal
					assertASTEqualNoLoc(t, value, actual, expected)
				})
			}

			for _, value := range fixture.Invalid {
				if value == "" {
					continue
				}
				t.Run(value, func(t *testing.T) {
					if _, err := Parse(":nth-child("+value+")", ParseOptions{Context: "selector"}); err == nil {
						t.Errorf("Parse(%q) = nil error, want an error", ":nth-child("+value+")")
					}
				})
			}
		})

		t.Run("UnicodeRange", func(t *testing.T) {
			fixture := genericTypesFixture["<urange>"]

			for _, value := range fixture.Valid {
				t.Run(value, func(t *testing.T) {
					ast, err := Parse(value, ParseOptions{Context: "value"})
					if err != nil {
						t.Fatalf("Parse(%q) error = %v", value, err)
					}
					actual := ast.(*Value).Children.First()
					expected := map[string]any{
						"type":  "UnicodeRange",
						"value": value,
					}

					// AST should be equal
					assertASTEqualNoLoc(t, value, actual, expected)
				})
			}

			for _, value := range fixture.Invalid {
				t.Run(value, func(t *testing.T) {
					// upstream: assert.throws(() => { parse(); assert AST equals Value[Raw] })
					actual, err := Parse(value, ParseOptions{Context: "value"})
					if err != nil {
						return
					}
					expected := map[string]any{
						"type": "Value",
						"children": []any{
							map[string]any{
								"type":  "Raw",
								"value": value,
							},
						},
					}

					// AST should not be equal
					if reflect.DeepEqual(stripLoc(toPlainJSON(t, actual)), stripLoc(toPlainJSON(t, expected))) {
						t.Errorf("Parse(%q) = Value[Raw %q], want an error or another AST", value, value)
					}
				})
			}
		})
	})

	t.Run("context", func(t *testing.T) {
		t.Run("should take parse context", func(t *testing.T) {
			ast, err := Parse("property: value", ParseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{
				"type": "StyleSheet",
				"loc":  nil,
				"children": []any{
					map[string]any{
						"type":  "Raw",
						"loc":   nil,
						"value": "property: value",
					},
				},
			}
			if got := toPlainJSON(t, ast); !reflect.DeepEqual(got, want) {
				t.Errorf("Parse(%q) = %v, want %v", "property: value", got, want)
			}

			ast, err = Parse("property: value", ParseOptions{
				Context: "declaration",
			})
			if err != nil {
				t.Fatal(err)
			}
			want = map[string]any{
				"type":      "Declaration",
				"loc":       nil,
				"important": false,
				"property":  "property",
				"value": map[string]any{
					"type": "Value",
					"loc":  nil,
					"children": []any{
						map[string]any{
							"type": "Identifier",
							"loc":  nil,
							"name": "value",
						},
					},
				},
			}
			if got := toPlainJSON(t, ast); !reflect.DeepEqual(got, want) {
				t.Errorf("Parse(%q, declaration) = %v, want %v", "property: value", got, want)
			}
		})

		t.Run("wrong context", func(t *testing.T) {
			_, err := Parse("a{}", ParseOptions{Context: "unknown"})
			if err == nil || !strings.Contains(err.Error(), "Unknown context `unknown`") {
				t.Errorf("Parse(%q, unknown) error = %v, want Unknown context `unknown`", "a{}", err)
			}
		})
	})

	t.Run("list", func(t *testing.T) {
		t.Run("should use List for children by default", func(t *testing.T) {
			ast, err := Parse("a{}", ParseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if ast.(*StyleSheet).Children == nil {
				t.Error("Children = nil, should be List")
			}
		})

		t.Run("should use List for children when list is true", func(t *testing.T) {
			t.Skip("not ported: children are always *List[Node], there is no list option")
		})

		t.Run("should use Array for children when list is false", func(t *testing.T) {
			t.Skip("not ported: children are always *List[Node], there is no list option")
		})
	})

	t.Run("should call onParseError when handler is passed", func(t *testing.T) {
		type parseError struct {
			err          error
			fallbackNode Node
		}
		var errs []parseError
		ast, err := Parse("{a: 1!; foo; b: 2}", ParseOptions{
			Context: "block",
			OnParseError: func(err error, fallbackNode Node) error {
				errs = append(errs, parseError{err, fallbackNode})
				return nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		if got := ast.(*Block).Children.Size(); got != 3 {
			t.Errorf("Children.Size() = %d, want 3", got)
		}
		if len(errs) != 2 {
			t.Fatalf("len(errors) = %d, want 2", len(errs))
		}
		// upstream: "Identifier is expected" with the Raw "a: 1!;" in place
		// of the declaration; the port follows CSS Syntax 3, where a ! not
		// followed by an identifier at the end is part of the value, so the
		// value 1! falls back to Raw
		if got := errs[0].err.Error(); got != "Unexpected input" {
			t.Errorf("errors[0].error.message = %q, want %q", got, "Unexpected input")
		}
		if got := errs[0].fallbackNode.(*Raw).Value; got != "1!" {
			t.Errorf("errors[0].fallbackNode.value = %q, want %q", got, "1!")
		}
		if got := errs[1].err.Error(); got != "Colon is expected" {
			t.Errorf("errors[1].error.message = %q, want %q", got, "Colon is expected")
		}
		if got := errs[1].fallbackNode.(*Raw).Value; got != "foo;" {
			t.Errorf("errors[1].fallbackNode.value = %q, want %q", got, "foo;")
		}
	})

	t.Run("errors", func(t *testing.T) {
		throwOnParseErrorOptions := ParseOptions{
			OnParseError: func(err error, _ Node) error {
				return err
			},
		}
		parseSyntaxError := func(t *testing.T, source string, options ParseOptions) *SyntaxError {
			t.Helper()
			_, err := Parse(source, options)
			var syntaxError *SyntaxError
			if !errors.As(err, &syntaxError) {
				t.Fatalf("Parse(%q) error = %v, want a *SyntaxError", source, err)
			}
			return syntaxError
		}
		checkString := func(t *testing.T, what, got, want string) {
			t.Helper()
			if got != want {
				t.Errorf("%s =\n%s\nwant\n%s", what, got, want)
			}
		}

		forEachASTTest(t, true, func(t *testing.T, test astFixtureTest) {
			options := test.ParseOptions(true)
			options.Positions = false
			createParseErrorTest(t, test.Name, test, options)
			options.Positions = true
			createParseErrorTest(t, test.Name+" (with positions)", test, options)
		})

		t.Run("formattedMessage", func(t *testing.T) {
			e := parseSyntaxError(t, "/**/\n.\nfoo", throwOnParseErrorOptions)
			checkString(t, "formattedMessage", e.FormattedMessage(),
				"Parse error: Identifier is expected\n"+
					"    1 |/**/\n"+
					"    2 |.\n"+
					"--------^\n"+
					"    3 |foo",
			)
			checkString(t, "sourceFragment()", e.SourceFragment(0),
				"    2 |.\n"+
					"--------^",
			)
			checkString(t, "sourceFragment(3)", e.SourceFragment(3),
				"    1 |/**/\n"+
					"    2 |.\n"+
					"--------^\n"+
					"    3 |foo",
			)
		})

		t.Run("formattedMessage at eof", func(t *testing.T) {
			e := parseSyntaxError(t, ".", throwOnParseErrorOptions)
			checkString(t, "formattedMessage", e.FormattedMessage(),
				"Parse error: Identifier is expected\n"+
					"    1 |.\n"+
					"--------^",
			)
		})

		t.Run("formattedMessage (windows new lines)", func(t *testing.T) {
			e := parseSyntaxError(t, "/**/\r\n.\r\nfoo", throwOnParseErrorOptions)
			checkString(t, "formattedMessage", e.FormattedMessage(),
				"Parse error: Identifier is expected\n"+
					"    1 |/**/\n"+
					"    2 |.\n"+
					"--------^\n"+
					"    3 |foo",
			)
			checkString(t, "sourceFragment()", e.SourceFragment(0),
				"    2 |.\n"+
					"--------^",
			)
			checkString(t, "sourceFragment(3)", e.SourceFragment(3),
				"    1 |/**/\n"+
					"    2 |.\n"+
					"--------^\n"+
					"    3 |foo",
			)
		})

		t.Run("formattedMessage with tabs", func(t *testing.T) {
			e := parseSyntaxError(t, "a {\n\tb:\tc#\t\n}", throwOnParseErrorOptions)
			checkString(t, "formattedMessage", e.FormattedMessage(),
				"Parse error: Hex or identifier is expected\n"+
					"    1 |a {\n"+
					"    2 |    b:    c#    \n"+
					"-------------------^\n"+
					"    3 |}",
			)
		})

		t.Run("formattedMessage for source with long lines", func(t *testing.T) {
			e := parseSyntaxError(t,
				"/*"+strings.Repeat("1234567890", 20)+"*/\n"+
					strings.Repeat(" ", 117)+".\n"+
					"foo\n"+
					strings.Repeat(" ", 120)+"bar",
				throwOnParseErrorOptions,
			)
			checkString(t, "formattedMessage", e.FormattedMessage(),
				"Parse error: Identifier is expected\n"+
					"    1 |…12345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678…\n"+
					"    2 |…                                                       .\n"+
					"----------------------------------------------------------------^\n"+
					"    3 |\n"+
					"    4 |…                                                          bar",
			)
		})

		t.Run("with custom offset/line/column", func(t *testing.T) {
			e := parseSyntaxError(t, "#.classname\n\n\n", ParseOptions{
				Context:   "selector",
				Positions: true,
				Offset:    10,
				Line:      10,
				Column:    10,
			})
			checkString(t, "formattedMessage", e.FormattedMessage(),
				"Parse error: Name is expected\n"+
					"   10 |         #.classname\n"+
					"-----------------^",
			)
			type errorFields struct {
				Source string
				Offset int
				Line   int
				Column int
			}
			want := errorFields{
				Source: "#.classname\n\n\n",
				Offset: 11,
				Line:   10,
				Column: 11,
			}
			got := errorFields{
				Source: e.Source,
				Offset: e.Offset,
				Line:   e.Line,
				Column: e.Column,
			}
			if got != want {
				t.Errorf("error = %+v, want %+v", got, want)
			}
		})
	})

	t.Run("onComment", func(t *testing.T) {
		const source = "/*123*/.foo[a=/* 234 */] {\n  color: red; /* 345*/\n  background: url(/*456*/foo);\n} /*567*"
		type comment struct {
			Value string
			Loc   *Location
		}

		t.Run("with no locations", func(t *testing.T) {
			var actual []comment
			if _, err := Parse(source, ParseOptions{
				OnComment: func(value string, loc *Location) {
					actual = append(actual, comment{value, loc})
				},
			}); err != nil {
				t.Fatal(err)
			}

			want := []comment{
				{Value: "123", Loc: nil},
				{Value: " 234 ", Loc: nil},
				{Value: " 345", Loc: nil},
				{Value: "567*", Loc: nil},
			}
			if !reflect.DeepEqual(actual, want) {
				t.Errorf("comments = %+v, want %+v", actual, want)
			}
		})

		t.Run("with locations", func(t *testing.T) {
			var actual []comment
			offsetToPos := func(offset int) tokenizer.Position {
				lines := strings.Split(source[:offset], "\n")
				return tokenizer.Position{
					Offset: offset,
					Line:   len(lines),
					Column: len(lines[len(lines)-1]) + 1,
				}
			}
			loc := func(start, end int) *Location {
				return &Location{
					Source: "test.css",
					Start:  offsetToPos(start),
					End:    offsetToPos(end),
				}
			}

			if _, err := Parse(source, ParseOptions{
				Filename:  "test.css",
				Positions: true,
				OnComment: func(value string, loc *Location) {
					actual = append(actual, comment{value, loc})
				},
			}); err != nil {
				t.Fatal(err)
			}

			want := []comment{
				{Value: "123", Loc: loc(0, 7)},
				{Value: " 234 ", Loc: loc(14, 23)},
				{Value: " 345", Loc: loc(41, 49)},
				{Value: "567*", Loc: loc(83, 89)},
			}
			if !reflect.DeepEqual(actual, want) {
				t.Errorf("comments = %+v, want %+v", actual, want)
			}
		})
	})

	t.Run("onToken", func(t *testing.T) {
		const source = "/*123*/.foo[a=/* 234 */] {\n  color: red;} /*567*"
		type token struct {
			Type  tokenizer.TokenType
			Start int
			End   int
		}
		expected := []token{
			{Type: tokenizer.Comment, Start: 0, End: 7},
			{Type: tokenizer.Delim, Start: 7, End: 8},
			{Type: tokenizer.Ident, Start: 8, End: 11},
			{Type: tokenizer.LeftSquareBracket, Start: 11, End: 12},
			{Type: tokenizer.Ident, Start: 12, End: 13},
			{Type: tokenizer.Delim, Start: 13, End: 14},
			{Type: tokenizer.Comment, Start: 14, End: 23},
			{Type: tokenizer.RightSquareBracket, Start: 23, End: 24},
			{Type: tokenizer.WhiteSpace, Start: 24, End: 25},
			{Type: tokenizer.LeftCurlyBracket, Start: 25, End: 26},
			{Type: tokenizer.WhiteSpace, Start: 26, End: 29},
			{Type: tokenizer.Ident, Start: 29, End: 34},
			{Type: tokenizer.Colon, Start: 34, End: 35},
			{Type: tokenizer.WhiteSpace, Start: 35, End: 36},
			{Type: tokenizer.Ident, Start: 36, End: 39},
			{Type: tokenizer.Semicolon, Start: 39, End: 40},
			{Type: tokenizer.RightCurlyBracket, Start: 40, End: 41},
			{Type: tokenizer.WhiteSpace, Start: 41, End: 42},
			{Type: tokenizer.Comment, Start: 42, End: 48},
		}

		t.Run("as function", func(t *testing.T) {
			type indexedToken struct {
				Index int
				token
			}
			var actual []indexedToken
			if _, err := Parse(source, ParseOptions{
				OnToken: func(_ *TokenIterateAPI, tokenType tokenizer.TokenType, start, end, index int) {
					actual = append(actual, indexedToken{index, token{tokenType, start, end}})
				},
			}); err != nil {
				t.Fatal(err)
			}

			var want []indexedToken
			for index, token := range expected {
				want = append(want, indexedToken{index, token})
			}
			if !slices.Equal(actual, want) {
				t.Errorf("tokens = %+v, want %+v", actual, want)
			}
		})

		t.Run("as function with API", func(t *testing.T) {
			const filename = "test.css"
			offsetToLocation := tokenizer.NewOffsetToLocation(source, 0, 1, 1)
			tokenStream := tokenizer.NewTokenStream(source, tokenizer.Tokenize)
			type apiToken struct {
				Index                  int
				Filename               string
				Source                 string
				TokenCount             int
				Type                   tokenizer.TokenType
				TypeName               string
				IsBlockOpenerTokenType bool
				IsBlockCloserTokenType bool
				Balance                int
				BlockTokenPairIndex    int
				Start                  int
				End                    int
				Value                  string
				Substring              string
				Location               tokenizer.SourcePosition
				RangeLocation          *Location
			}
			var actual []apiToken

			if _, err := Parse(source, ParseOptions{
				Filename: filename,
				OnToken: func(api *TokenIterateAPI, tokenType tokenizer.TokenType, start, end, index int) {
					actual = append(actual, apiToken{
						Index:                  index,
						Filename:               api.Filename,
						Source:                 api.Source,
						TokenCount:             api.TokenCount,
						Type:                   api.GetTokenType(index),
						TypeName:               api.GetTokenTypeName(index),
						IsBlockOpenerTokenType: api.IsBlockOpenerTokenType(tokenType),
						IsBlockCloserTokenType: api.IsBlockCloserTokenType(tokenType),
						Balance:                api.Balance[index],
						BlockTokenPairIndex:    api.GetBlockTokenPairIndex(index),
						Start:                  api.GetTokenStart(index),
						End:                    api.GetTokenEnd(index),
						Value:                  api.GetTokenValue(index),
						Substring:              api.Substring(start, end),
						Location:               api.GetLocation(start),
						RangeLocation:          api.GetRangeLocation(start, end),
					})
				},
			}); err != nil {
				t.Fatal(err)
			}

			var want []apiToken
			for index, token := range expected {
				want = append(want, apiToken{
					Index:                  index,
					Filename:               filename,
					Source:                 source,
					TokenCount:             tokenStream.TokenCount,
					Type:                   token.Type,
					TypeName:               tokenizer.TokenNames[token.Type],
					IsBlockOpenerTokenType: slices.Contains([]tokenizer.TokenType{tokenizer.Function, tokenizer.LeftParenthesis, tokenizer.LeftSquareBracket, tokenizer.LeftCurlyBracket}, token.Type),
					IsBlockCloserTokenType: slices.Contains([]tokenizer.TokenType{tokenizer.RightParenthesis, tokenizer.RightSquareBracket, tokenizer.RightCurlyBracket}, token.Type),
					Balance:                tokenStream.Balance[index],
					BlockTokenPairIndex:    tokenStream.GetBlockTokenPairIndex(index),
					Start:                  token.Start,
					End:                    token.End,
					Value:                  source[token.Start:token.End],
					Substring:              source[token.Start:token.End],
					Location:               offsetToLocation.GetLocation(token.Start, filename),
					RangeLocation:          offsetToLocation.GetLocationRange(token.Start, token.End, filename),
				})
			}
			if !reflect.DeepEqual(actual, want) {
				t.Errorf("tokens =\n%+v\nwant\n%+v", actual, want)
			}
		})

		t.Run("as array", func(t *testing.T) {
			t.Skip("not ported: onToken takes only the function form")
		})
	})

	t.Run("positions", func(t *testing.T) {
		const source = ".foo.bar {\n  property: value 123 123.4 .123 123px 99% #fff url( a ) / var( --a ), \"test\" 'test';\n}"

		type position struct {
			Offset, Line, Column int
			Type                 string
		}

		collectPositions := func(t *testing.T, options ParseOptions) []position {
			t.Helper()
			var positions []position
			ast, err := Parse(source, options)
			if err != nil {
				t.Fatal(err)
			}

			if err := Walk(ast, WalkOptions{
				Enter: func(_ *WalkContext, node Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
					if loc := node.Location(); loc != nil {
						positions = append(positions, position{
							loc.Start.Offset,
							loc.Start.Line,
							loc.Start.Column,
							node.Type(),
						})
					}
					return WalkContinue
				},
			}); err != nil {
				t.Fatal(err)
			}
			return positions
		}

		t.Run("should start with line 1 column 1 by default", func(t *testing.T) {
			positions := collectPositions(t, ParseOptions{
				Positions: true,
			})

			expected := []position{
				{0, 1, 1, "StyleSheet"},
				{0, 1, 1, "Rule"},
				{0, 1, 1, "SelectorList"},
				{0, 1, 1, "Selector"},
				{0, 1, 1, "ClassSelector"},
				{4, 1, 5, "ClassSelector"},
				{9, 1, 10, "Block"},
				{13, 2, 3, "Declaration"},
				{23, 2, 13, "Value"},
				{23, 2, 13, "Identifier"},
				{29, 2, 19, "Number"},
				{33, 2, 23, "Number"},
				{39, 2, 29, "Number"},
				{44, 2, 34, "Dimension"},
				{50, 2, 40, "Percentage"},
				{54, 2, 44, "Hash"},
				{59, 2, 49, "Url"},
				{68, 2, 58, "Operator"},
				{70, 2, 60, "Function"},
				{75, 2, 65, "Identifier"},
				{80, 2, 70, "Operator"},
				{82, 2, 72, "String"},
				{89, 2, 79, "String"},
			}
			if !slices.Equal(positions, expected) {
				t.Errorf("positions =\n%v\nwant\n%v", positions, expected)
			}
		})

		t.Run("should start with specified offset, line and column", func(t *testing.T) {
			positions := collectPositions(t, ParseOptions{
				Positions: true,
				Offset:    100,
				Line:      3,
				Column:    5,
			})

			expected := []position{
				{100, 3, 5, "StyleSheet"},
				{100, 3, 5, "Rule"},
				{100, 3, 5, "SelectorList"},
				{100, 3, 5, "Selector"},
				{100, 3, 5, "ClassSelector"},
				{104, 3, 9, "ClassSelector"},
				{109, 3, 14, "Block"},
				{113, 4, 3, "Declaration"},
				{123, 4, 13, "Value"},
				{123, 4, 13, "Identifier"},
				{129, 4, 19, "Number"},
				{133, 4, 23, "Number"},
				{139, 4, 29, "Number"},
				{144, 4, 34, "Dimension"},
				{150, 4, 40, "Percentage"},
				{154, 4, 44, "Hash"},
				{159, 4, 49, "Url"},
				{168, 4, 58, "Operator"},
				{170, 4, 60, "Function"},
				{175, 4, 65, "Identifier"},
				{180, 4, 70, "Operator"},
				{182, 4, 72, "String"},
				{189, 4, 79, "String"},
			}
			if !slices.Equal(positions, expected) {
				t.Errorf("positions =\n%v\nwant\n%v", positions, expected)
			}
		})
	})
}
