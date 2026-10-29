// Ported from lib/__tests/decode-encode.js.
//
// JavaScript strings are UTF-16; the Go inputs are their UTF-8 encodings
// (U+0080..U+00FF become two bytes). The lone surrogates of the ident
// encode tests are written as their WTF-8 bytes, which pass through
// EncodeIdent unchanged like every other byte >= 0x80.

package csstree

import (
	"fmt"
	"testing"
)

type decodeEncodeTest struct {
	from string
	to   string
}

func forEachDecodeEncodeTest(t *testing.T, tests []decodeEncodeTest, fn func(string) string) {
	for idx, test := range tests {
		t.Run(fmt.Sprintf("(%d) %s", idx, jsonString(test.from)), func(t *testing.T) {
			if got := fn(test.from); got != test.to {
				t.Errorf("(%q) = %q, want %q", test.from, got, test.to)
			}
		})
	}
}

func TestDecodeEncode(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		t.Run("decode", func(t *testing.T) {
			tests := []decodeEncodeTest{
				{``, ``},
				{`"`, ``},
				{`'`, ``},
				{`""`, ``},
				{`''`, ``},
				// (5)
				{`"'`, `'`},
				{`'"`, `"`},
				{`"\""`, `"`},
				{`"'"`, `'`},
				{`"\"`, `"`},
				// (10)
				{`'\'`, `'`},
				{`"\`, ``},
				{`'\`, ``},
				{"\"a\\\nb\"", `ab`},
				{"\"a\\\rb\"", `ab`},
				// (15)
				{"\"a\\\fb\"", `ab`},
				{`"\21"`, `!`},
				{`"\021"`, `!`},
				{`"\0021"`, `!`},
				{`"\00021"`, `!`},
				// (20)
				{`"\000021"`, `!`},
				{`"\0000211"`, `!1`},
				{`"\000021 1"`, `!1`},
				{"\"\\000021\t1\"", `!1`},
				{`"\0"`, "\ufffd"},
				// (25)
				{`"\0x"`, "\ufffdx"},
				{`"\abcdefa"`, "\ufffda"}, // is greater than the maximum allowed code point
				{`"\def0"`, "\ufffd"},     // is for a surrogate
				{`"\00abcdef"`, "\uabcdef"},
				{`"\abcdef1"`, "\ufffd1"},
				// (30)
				{`"\a\d\c\9"`, "\n\r\f\t"},
				{`"\(\)\\"`, `()\`},
				{"\"\\\r\\\n\\\r\n\"", ``},
				{`"\"`, `"`},
				{`"\`, ``},
				// (35)
				{`\31  b`, `1 b`},
			}

			forEachDecodeEncodeTest(t, tests, DecodeString)
		})

		t.Run("encode", func(t *testing.T) {
			tests := []decodeEncodeTest{
				{``, `""`},
				{`"`, `"\""`},
				{`'`, `"'"`},
				{"a\nb", `"a\a b"`},
				{"a\nz", `"a\az"`},
				// (5)
				{"a\rb", `"a\d b"`},
				{"a\fb", `"a\c b"`},
				{"a\tb", `"a\9 b"`},
				{"a\nbc\n\"b\tx", `"a\a bc\a\"b\9x"`},
				{`a\26b`, `"a\\26b"`},
				// (10)
				{`a&b`, `"a&b"`},
				{`a&z`, `"a&z"`},
				{"\n b", `"\a  b"`},
			}

			forEachDecodeEncodeTest(t, tests, func(s string) string {
				return EncodeString(s, false)
			})
		})
	})

	t.Run("url", func(t *testing.T) {
		t.Run("decode", func(t *testing.T) {
			tests := []decodeEncodeTest{
				{`url(`, ``},
				{`url(foo`, `foo`},
				{`url(foo)`, `foo`},
				{`url(  foo  )`, `foo`},
				{`url(  1\ \(2\).jpg  )`, `1 (2).jpg`},
				// (5)
				{`url(  \"\'\(\)\ \\ )`, `"'() \`},
				{"url(  a\\\r\\\n\\\r\nb  )", `ab`},
				{`url(\21)`, `!`},
				{`url(\021)`, `!`},
				{`url(\0021)`, `!`},
				// (10)
				{`url(\00021)`, `!`},
				{`url(\000021)`, `!`},
				{`url(\0000211)`, `!1`},
				{`url(\000021 1)`, `!1`},
				{"url(\\000021\t1)", `!1`},
				// (15)
				{`url(\0)`, "\ufffd"},
				{`url(\0x)`, "\ufffdx"},
				{`url(\abcdefa)`, "\ufffda"}, // is greater than the maximum allowed code point
				{`url(\def0)`, "\ufffd"},     // is for a surrogate
				{`url(\00abcdef)`, "\uabcdef"},
				// (20)
				{`url(\abcdef1)`, "\ufffd1"},
				{`url(\)`, `)`},
				{`url(\`, ``},
			}

			forEachDecodeEncodeTest(t, tests, DecodeURL)
		})

		t.Run("encode", func(t *testing.T) {
			tests := []decodeEncodeTest{
				{``, `url()`},
				{`"`, `url(\")`},
				{`'`, `url(\')`},
				{"a\nb", `url(a\a b)`},
				{"a\nz", `url(a\az)`},
				// (5)
				{"a\rb", `url(a\d b)`},
				{"a\fb", `url(a\c b)`},
				{"a\tb", `url(a\9 b)`},
				{"a\nbc\n\"b\tx", `url(a\a bc\a\"b\9x)`},
				{`a\26b`, `url(a\\26b)`},
				// (10)
				{`1 (2).jpg`, `url(1\ \(2\).jpg)`},
				{`"'() \`, `url(\"\'\(\)\ \\)`},
				{"\u0008", `url(\8)`},
				{`1 b`, `url(1\ b)`},
				{"1\n b", `url(1\a\ b)`},
			}

			forEachDecodeEncodeTest(t, tests, EncodeURL)
		})
	})

	t.Run("ident", func(t *testing.T) {
		t.Run("decode", func(t *testing.T) {
			tests := []decodeEncodeTest{
				{``, ``},
				{`foo`, `foo`},
				{"a\\\r\\\n\\\r\nb", `ab`},
				{`\21`, `!`},
				{`\021`, `!`},
				// (5)
				{`\0021`, `!`},
				{`\00021`, `!`},
				{`\000021`, `!`},
				{`\0000211`, `!1`},
				{`\000021 1`, `!1`},
				// (10)
				{"\\000021\t1", `!1`},
				{`\0`, "\ufffd"},
				{`\0x`, "\ufffdx"},
				{`\abcdefa`, "\ufffda"}, // is greater than the maximum allowed code point
				{`\def0`, "\ufffd"},     // is for a surrogate
				// (15)
				{`\00abcdef`, "\uabcdef"},
				{`\abcdef1`, "\ufffd1"},
				{`\`, ``},
				{`\31 \ b`, `1 b`},
				{`\31 \ x`, `1 x`},
				// (20)
				{`\31 b`, `1b`},
				{`\31 x`, `1x`},
				{`\31 `, `1`},
			}

			forEachDecodeEncodeTest(t, tests, DecodeIdent)
		})

		t.Run("encode", func(t *testing.T) {
			// Adopted tests: https://github.com/mathiasbynens/CSS.escape/blob/master/tests/tests.js
			tests := []decodeEncodeTest{
				{``, ``},
				{"\x00", "\uFFFD"},
				{"a\x00", "a\uFFFD"},
				{"\x00b", "\uFFFDb"},
				{"a\x00b", "a\uFFFDb"},

				{"\uFFFD", "\uFFFD"},
				{"a\uFFFD", "a\uFFFD"},
				{"\uFFFDb", "\uFFFDb"},
				{"a\uFFFDb", "a\uFFFDb"},

				{"\x01\x02\x1E\x1F", `\1 \2 \1e \1f `},

				// (10)
				{`0a`, `\30 a`},
				{`1a`, `\31 a`},
				{`2a`, `\32 a`},
				{`3a`, `\33 a`},
				{`4a`, `\34 a`},
				{`5a`, `\35 a`},
				{`6a`, `\36 a`},
				{`7a`, `\37 a`},
				{`8a`, `\38 a`},
				{`9a`, `\39 a`},

				// (20)
				{`a0b`, `a0b`},
				{`a1b`, `a1b`},
				{`a2b`, `a2b`},
				{`a3b`, `a3b`},
				{`a4b`, `a4b`},
				{`a5b`, `a5b`},
				{`a6b`, `a6b`},
				{`a7b`, `a7b`},
				{`a8b`, `a8b`},
				{`a9b`, `a9b`},

				// (30)
				{`-0a`, `-\30 a`},
				{`-1a`, `-\31 a`},
				{`-2a`, `-\32 a`},
				{`-3a`, `-\33 a`},
				{`-4a`, `-\34 a`},
				{`-5a`, `-\35 a`},
				{`-6a`, `-\36 a`},
				{`-7a`, `-\37 a`},
				{`-8a`, `-\38 a`},
				{`-9a`, `-\39 a`},

				// (40)
				{`-`, `\-`},
				{`-a`, `-a`},
				{`--`, `--`},
				{`--a`, `--a`},

				{"\u0080\x2D\x5F\u00A9", "\u0080\x2D\x5F\u00A9"},
				{
					"\x7F\u0080\u0081\u0082\u0083\u0084\u0085\u0086\u0087\u0088\u0089\u008A\u008B\u008C\u008D\u008E\u008F\u0090\u0091\u0092\u0093\u0094\u0095\u0096\u0097\u0098\u0099\u009A\u009B\u009C\u009D\u009E\u009F",
					"\\7f \u0080\u0081\u0082\u0083\u0084\u0085\u0086\u0087\u0088\u0089\u008A\u008B\u008C\u008D\u008E\u008F\u0090\u0091\u0092\u0093\u0094\u0095\u0096\u0097\u0098\u0099\u009A\u009B\u009C\u009D\u009E\u009F",
				},
				{"\u00A0\u00A1\u00A2", "\u00A0\u00A1\u00A2"},
				{`a0123456789b`, `a0123456789b`},
				{`abcdefghijklmnopqrstuvwxyz`, `abcdefghijklmnopqrstuvwxyz`},
				{`ABCDEFGHIJKLMNOPQRSTUVWXYZ`, `ABCDEFGHIJKLMNOPQRSTUVWXYZ`},

				// (50)
				{"\x20\x21\x78\x79", `\ \!xy`},

				// astral symbol (U+1D306 TETRAGRAM FOR CENTRE)
				{"\U0001D306", "\U0001D306"},
				// lone surrogates (WTF-8 bytes of U+DF06 and U+D834)
				{"\xed\xbc\x86", "\xed\xbc\x86"},
				{"\xed\xa0\xb4", "\xed\xa0\xb4"},

				{`1 b`, `\31 \ b`},
				// (55)
				{`1 x`, `\31 \ x`},
				{`1b`, `\31 b`},
				{`1x`, `\31 x`},
				{`1`, `\31 `},
			}

			forEachDecodeEncodeTest(t, tests, EncodeIdent)
		})
	})
}
