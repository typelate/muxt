// Ported from lib/__tests/tokenizer.js and lib/__tests/fixture/tokenize.js.

package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type tokenChunk struct {
	Type  string `json:"type"`
	Chunk string `json:"chunk"`
}

type testToken struct {
	Type    string
	Chunk   string
	Balance int
}

func TestTokenizeStream(t *testing.T) {
	t.Run("tokenize/stream", func(t *testing.T) {
		createStream := func(source string) *TokenStream {
			return NewTokenStream(source, Tokenize)
		}
		const css = ".test\n{\n  prop: url(foo/bar.jpg) url( a\\(\\33 \\).\\ \\\"\\'test ) calc(1 + 1) \\x \\aa ;\n}<!--<-->\\\n"
		tokens := []testToken{
			{Type: "delim-token", Chunk: ".", Balance: 31},
			{Type: "ident-token", Chunk: "test", Balance: 31},
			{Type: "whitespace-token", Chunk: "\n", Balance: 31},
			{Type: "{-token", Chunk: "{", Balance: 25},
			{Type: "whitespace-token", Chunk: "\n  ", Balance: 25},
			{Type: "ident-token", Chunk: "prop", Balance: 25},
			{Type: "colon-token", Chunk: ":", Balance: 25},
			{Type: "whitespace-token", Chunk: " ", Balance: 25},
			{Type: "url-token", Chunk: "url(foo/bar.jpg)", Balance: 25},
			{Type: "whitespace-token", Chunk: " ", Balance: 25},
			{Type: "url-token", Chunk: "url( a\\(\\33 \\).\\ \\\"\\'test )", Balance: 25},
			{Type: "whitespace-token", Chunk: " ", Balance: 25},
			{Type: "function-token", Chunk: "calc(", Balance: 18},
			{Type: "number-token", Chunk: "1", Balance: 18},
			{Type: "whitespace-token", Chunk: " ", Balance: 18},
			{Type: "delim-token", Chunk: "+", Balance: 18},
			{Type: "whitespace-token", Chunk: " ", Balance: 18},
			{Type: "number-token", Chunk: "1", Balance: 18},
			{Type: ")-token", Chunk: ")", Balance: 12},
			{Type: "whitespace-token", Chunk: " ", Balance: 25},
			{Type: "ident-token", Chunk: "\\x", Balance: 25},
			{Type: "whitespace-token", Chunk: " ", Balance: 25},
			{Type: "ident-token", Chunk: "\\aa ", Balance: 25},
			{Type: "semicolon-token", Chunk: ";", Balance: 25},
			{Type: "whitespace-token", Chunk: "\n", Balance: 25},
			{Type: "}-token", Chunk: "}", Balance: 3},
			{Type: "CDO-token", Chunk: "<!--", Balance: 31},
			{Type: "delim-token", Chunk: "<", Balance: 31},
			{Type: "CDC-token", Chunk: "-->", Balance: 31},
			{Type: "delim-token", Chunk: "\\", Balance: 31},
			{Type: "whitespace-token", Chunk: "\n", Balance: 31},
		}
		var (
			dump  []DumpToken
			types []string
			start []int
			end   []int
		)
		offset := 0
		for idx, token := range tokens {
			dump = append(dump, DumpToken{Idx: idx, Type: token.Type, Chunk: token.Chunk, Balance: token.Balance})
			types = append(types, token.Type)
			start = append(start, offset)
			offset += len(token.Chunk)
			end = append(end, offset)
		}

		t.Run("edge case: no arguments", func(t *testing.T) {
			stream := NewTokenStream("", nil)

			if !stream.EOF {
				t.Errorf("stream.EOF = false, want true")
			}
			if stream.TokenType != 0 {
				t.Errorf("stream.TokenType = %d, want 0", stream.TokenType)
			}
			if stream.Source != "" {
				t.Errorf("stream.Source = %q, want %q", stream.Source, "")
			}
		})

		t.Run("edge case: empty input", func(t *testing.T) {
			stream := createStream("")

			if !stream.EOF {
				t.Errorf("stream.EOF = false, want true")
			}
			if stream.TokenType != 0 {
				t.Errorf("stream.TokenType = %d, want 0", stream.TokenType)
			}
			if stream.Source != "" {
				t.Errorf("stream.Source = %q, want %q", stream.Source, "")
			}
		})

		t.Run("should convert input to string", func(t *testing.T) {
			t.Skip("not ported: Go source is typed as string, there is no toString() conversion")
		})

		t.Run("should accept a Buffer", func(t *testing.T) {
			t.Skip("not ported: Go source is typed as string, there is no Buffer conversion")
		})

		t.Run("dump()", func(t *testing.T) {
			stream := createStream(css)

			if got := stream.Dump(); !slices.Equal(got, dump) {
				t.Errorf("Dump() = %v, want %v", got, dump)
			}
		})

		t.Run("next() types", func(t *testing.T) {
			stream := createStream(css)
			var actual []string

			for !stream.EOF {
				actual = append(actual, TokenNames[stream.TokenType])
				stream.Next()
			}

			if !slices.Equal(actual, types) {
				t.Errorf("types = %q, want %q", actual, types)
			}
		})

		t.Run("next() start", func(t *testing.T) {
			stream := createStream(css)
			var actual []int

			for !stream.EOF {
				actual = append(actual, stream.TokenStart)
				stream.Next()
			}

			if !slices.Equal(actual, start) {
				t.Errorf("starts = %v, want %v", actual, start)
			}
		})

		t.Run("next() end", func(t *testing.T) {
			stream := createStream(css)
			var actual []int

			for !stream.EOF {
				actual = append(actual, stream.TokenEnd)
				stream.Next()
			}

			if !slices.Equal(actual, end) {
				t.Errorf("ends = %v, want %v", actual, end)
			}
		})

		t.Run("skip()", func(t *testing.T) {
			stream := createStream(css)
			var targetTokens []int // indices into tokens
			for i, token := range tokens {
				if token.Type == "ident-token" || token.Type == "delim-token" {
					targetTokens = append(targetTokens, i)
				}
			}
			var actual, want []string
			for idx, tokenIndex := range targetTokens {
				skip := tokenIndex
				if idx > 0 {
					skip = tokenIndex - targetTokens[idx-1]
				}
				stream.Skip(skip)
				actual = append(actual, TokenNames[stream.TokenType])
				want = append(want, tokens[tokenIndex].Type)
			}

			if len(actual) != 8 { // 4 x Indentifier + 4 x delim-token
				t.Errorf("len(actual) = %d, want 8", len(actual))
			}
			if !slices.Equal(actual, want) {
				t.Errorf("types = %q, want %q", actual, want)
			}
		})

		t.Run("skip() to end", func(t *testing.T) {
			stream := createStream(css)

			stream.Skip(len(tokens))

			if !stream.EOF {
				t.Errorf("stream.EOF = false, want true")
			}
		})

		t.Run("getBlockTokenPairIndex()", func(t *testing.T) {
			for _, tt := range []struct {
				name   string
				source string
				want   []int
			}{
				{name: "all blocks closed", source: "start fn({[()]}) end", want: []int{-1, -1, 9, 8, 7, 6, 5, 4, 3, 2, -1, -1}},
				{name: "non-closed blocks", source: "start fn(]}()[{)", want: []int{-1, -1, -1, -1, -1, 6, 5, -1, -1, -1}},
			} {
				t.Run(tt.name, func(t *testing.T) {
					stream := createStream(tt.source)
					var actual []int

					stream.ForEachToken(func(_ TokenType, _, _, index int) {
						actual = append(actual, stream.GetBlockTokenPairIndex(index))
					})

					if !slices.Equal(actual, tt.want) {
						t.Errorf("GetBlockTokenPairIndex over %q = %v, want %v", tt.source, actual, tt.want)
					}
				})
			}
		})

		t.Run("Raw", func(t *testing.T) {
			const (
				leftCurlyBracketCode = 0x007B // U+007B LEFT CURLY BRACKET ({)
				semicolonCode        = 0x003B // U+003B SEMICOLON (;)
			)
			leftCurlyBracket := func(code int) int {
				if code == leftCurlyBracketCode {
					return 1
				}
				return 0
			}
			semicolonIncluded := func(code int) int {
				if code == semicolonCode {
					return 2
				}
				return 0
			}
			tests := []struct {
				source   string
				start    string
				skip     string
				mode     func(code int) int
				expected string
			}{
				{
					source:   "? { }",
					start:    "^",
					skip:     "^",
					mode:     leftCurlyBracket,
					expected: "? ",
				},
				{
					// issues #56
					source:   "div { }",
					start:    "^",
					skip:     "^",
					mode:     leftCurlyBracket,
					expected: "div ",
				},
				{
					source:   "foo(bar(1)(2)(3[{}])(4{}){}(5))",
					start:    "             ^",
					skip:     "             ^",
					mode:     leftCurlyBracket,
					expected: "(3[{}])(4{})",
				},
				{
					source:   "foo(bar(1) (2) (3[{}]) (4{}) {} (5))",
					start:    "               ^",
					skip:     "                ^",
					mode:     leftCurlyBracket,
					expected: "(3[{}]) (4{}) ",
				},
				{
					source:   "func(a func(;))",
					start:    "     ^",
					skip:     "       ^",
					mode:     semicolonIncluded,
					expected: "a func(;)",
				},
				{
					source:   "func(a func(;))",
					start:    "     ^",
					skip:     "            ^",
					mode:     semicolonIncluded,
					expected: "a func(;)",
				},
				{
					source:   "func(a func(;); b)",
					start:    "     ^",
					skip:     "       ^",
					mode:     semicolonIncluded,
					expected: "a func(;);",
				},
				{
					source:   "func()",
					start:    "     ^",
					skip:     "     ^",
					mode:     nil,
					expected: "",
				},
				{
					source:   "func([{}])",
					start:    "      ^",
					skip:     "       ^",
					mode:     nil,
					expected: "{}",
				},
				{
					source:   "func([{})",
					start:    "     ^",
					skip:     "      ^",
					mode:     nil,
					expected: "[{})",
				},
				{
					source:   "func(1, 2, 3) {}",
					start:    "^",
					skip:     "      ^",
					mode:     nil,
					expected: "func(1, 2, 3) {}",
				},
			}

			for idx, test := range tests {
				t.Run(fmt.Sprintf("testcase#%d", idx), func(t *testing.T) {
					stream := createStream(test.source)
					startOffset := strings.Index(test.start, "^")
					skipToOffset := strings.Index(test.skip, "^")
					startToken := stream.TokenIndex

					for stream.TokenStart < startOffset {
						stream.Next()
						startToken = stream.TokenIndex
					}

					for stream.TokenStart < skipToOffset {
						stream.Next()
					}

					mode := test.mode
					if mode == nil {
						mode = func(int) int { return 0 }
					}
					stream.SkipUntilBalanced(startToken, mode)
					if got := stream.Source[startOffset:stream.TokenStart]; got != test.expected {
						t.Errorf("SkipUntilBalanced on %q consumed %q, want %q", test.source, got, test.expected)
					}
				})
			}
		})

		t.Run("dynamic buffer", func(t *testing.T) {
			bufferSize := len(createStream(css).offsetAndType) + 10
			stream := createStream(strings.Repeat(".", bufferSize))
			count := 0

			for !stream.EOF {
				count++
				stream.Next()
			}

			if count != bufferSize {
				t.Errorf("token count = %d, want %d", count, bufferSize)
			}
			if len(stream.offsetAndType) < bufferSize {
				t.Errorf("len(offsetAndType) = %d, want >= %d", len(stream.offsetAndType), bufferSize)
			}
		})

		t.Run("values", func(t *testing.T) {
			for _, testType := range []string{"valid", "invalid"} {
				forEachTokenizeTest(t, testType, func(name, value string, expected []tokenChunk) {
					t.Run(name, func(t *testing.T) {
						actual := []tokenChunk{}

						Tokenize(value, func(tokenType TokenType, start, end int) {
							actual = append(actual, tokenChunk{
								Type:  TokenNames[tokenType],
								Chunk: value[start:end],
							})
						})

						equal := slices.Equal(actual, expected)
						if testType == "valid" && !equal {
							t.Errorf("Tokenize(%q) = %v, want %v", value, actual, expected)
						}
						if testType == "invalid" && equal {
							t.Errorf("Tokenize(%q) = %v, want anything else", value, actual)
						}
					})
				})
			}
		})
	})
}

// tokenizeFixture is one entry of a testdata/tokenize/*.json file after
// processTests: a string value is shorthand for a single token whose type is
// the file's base name.
type tokenizeFixture struct {
	name   string
	value  string
	tokens []tokenChunk
}

type tokenizeFixtureFile struct {
	valid   []tokenizeFixture
	invalid []tokenizeFixture
}

// forEachTokenizeTest ports forEachTest from lib/__tests/fixture/tokenize.js.
// Test names use "<file> (<key> #<index>)" without the line:column prefix that
// upstream's JsonLocator adds.
func forEachTokenizeTest(t *testing.T, testType string, factory func(name, value string, expected []tokenChunk)) {
	t.Helper()
	for _, file := range loadTokenizeFixtures(t) {
		tests := file.valid
		if testType == "invalid" {
			tests = file.invalid
		}
		for _, test := range tests {
			factory(test.name, test.value, test.tokens)
		}
	}
}

func loadTokenizeFixtures(t *testing.T) []tokenizeFixtureFile {
	t.Helper()
	const dir = "../testdata/tokenize"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []tokenizeFixtureFile
	for _, entry := range entries {
		filename := entry.Name()
		if filepath.Ext(filename) != ".json" || entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			t.Fatal(err)
		}
		var raw map[string][]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("%s: %v", filename, err)
		}
		typ := strings.TrimSuffix(filename, ".json")
		files = append(files, tokenizeFixtureFile{
			valid:   processTokenizeTests(t, raw, "valid", typ, filename),
			invalid: processTokenizeTests(t, raw, "invalid", typ, filename),
		})
	}
	return files
}

func processTokenizeTests(t *testing.T, tests map[string][]json.RawMessage, key, typ, filename string) []tokenizeFixture {
	t.Helper()
	var result []tokenizeFixture
	for idx, raw := range tests[key] {
		name := fmt.Sprintf("%s (%s #%d)", filepath.ToSlash(filepath.Join("fixtures/tokenize", filename)), key, idx)
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			result = append(result, tokenizeFixture{name: name, value: value, tokens: []tokenChunk{{Type: typ, Chunk: value}}})
			continue
		}
		var object struct {
			Value  string       `json:"value"`
			Tokens []tokenChunk `json:"tokens"`
		}
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		result = append(result, tokenizeFixture{name: name, value: object.Value, tokens: object.Tokens})
	}
	return result
}

// TestPortDeviations covers the places where the Go port differs from
// upstream: BOM length, buffer reuse, byte columns and escape decoding. It has
// no upstream counterpart.
func TestPortDeviations(t *testing.T) {
	t.Run("BOM is skipped as 3 bytes", func(t *testing.T) {
		const source = "\uFEFFa b"
		var starts []int
		Tokenize(source, func(_ TokenType, start, _ int) {
			starts = append(starts, start)
		})
		if want := []int{3, 4, 5}; !slices.Equal(starts, want) {
			t.Errorf("Tokenize(%q) starts = %v, want %v", source, starts, want)
		}

		loc := NewOffsetToLocation(source, 0, 1, 1).GetLocationRange(3, 5, "x.css")
		want := &Location{Source: "x.css", Start: Position{Offset: 3, Line: 1, Column: 1}, End: Position{Offset: 5, Line: 1, Column: 3}}
		if *loc != *want {
			t.Errorf("GetLocationRange(3, 5) = %+v, want %+v", *loc, *want)
		}
	})

	t.Run("OffsetToLocation reuses a buffer as long as the source", func(t *testing.T) {
		l := NewOffsetToLocation("a", 0, 1, 1)
		l.GetLocation(0, "")
		source := strings.Repeat("a", len(l.lines))
		l.SetSource(source, 10, 2, 5)
		got := l.GetLocation(len(source), "")
		want := SourcePosition{Position: Position{Offset: 10 + len(source), Line: 2, Column: 5 + len(source)}}
		if got != want {
			t.Errorf("GetLocation(%d) = %+v, want %+v", len(source), got, want)
		}
	})

	t.Run("columns count bytes", func(t *testing.T) {
		const source = "é\nab"
		got := NewOffsetToLocation(source, 0, 1, 1).GetLocation(len(source), "")
		if want := (Position{Offset: 5, Line: 2, Column: 3}); got.Position != want {
			t.Errorf("GetLocation(%d) = %+v, want %+v", len(source), got.Position, want)
		}
	})

	t.Run("DecodeEscaped", func(t *testing.T) {
		for _, tt := range []struct {
			escaped, want string
		}{
			{escaped: "x", want: "x"},
			{escaped: "31 ", want: "1"},
			{escaped: "e9", want: "é"},
			{escaped: "0", want: "�"},
			{escaped: "D800", want: "�"},
			{escaped: "110000", want: "�"},
			{escaped: "FFFFFFFFFFFF", want: "�"},
			{escaped: "", want: "�"},
		} {
			if got := DecodeEscaped(tt.escaped); got != tt.want {
				t.Errorf("DecodeEscaped(%q) = %q, want %q", tt.escaped, got, tt.want)
			}
		}
	})
}
