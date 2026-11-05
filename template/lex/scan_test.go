package lex

import "testing"

type lexTest struct {
	name  string
	input string
	items []Token
}

var (
	tDot        = Token{Type: TokenTypeDot, Value: "."}
	tBlock      = Token{Type: TokenTypeBlock, Value: "block"}
	tEOF        = Token{Type: TokenTypeEOF, Value: ""}
	tFor        = Token{Type: TokenTypeIdentifier, Value: "for"}
	tLeft       = Token{Type: TokenTypeLeftDelim, Value: "{{"}
	tLpar       = Token{Type: TokenTypeLeftParen, Value: "("}
	tPipe       = Token{Type: TokenTypePipe, Value: "|"}
	tQuote      = Token{Type: TokenTypeString, Value: `"abc \n\t\" "`}
	tRange      = Token{Type: TokenTypeRange, Value: "range"}
	tRight      = Token{Type: TokenTypeRightDelim, Value: "}}"}
	tRpar       = Token{Type: TokenTypeRightParen, Value: ")"}
	tSpace      = Token{Type: TokenTypeSpace, Value: " "}
	raw         = "`" + `abc\n\t\" ` + "`"
	rawNL       = "`now is{{\n}}the time`" // Contains newline inside raw quote.
	tRawQuote   = Token{Type: TokenTypeRawString, Value: raw}
	tRawQuoteNL = Token{Type: TokenTypeRawString, Value: rawNL}
)

// collect gathers the emitted items into a slice.
func collect(t *lexTest, left, right string) (items []Token) {
	l := New(t.input, Options{
		LeftDelim:   left,
		RightDelim:  right,
		EmitComment: true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	for {
		item := l.Next()
		items = append(items, item)
		if item.Type == TokenTypeEOF || item.Type == TokenTypeError {
			break
		}
	}
	return
}

func equalSkipPosCheck(i1, i2 []Token) bool {
	if len(i1) != len(i2) {
		return false
	}
	for k := range i1 {
		if i1[k].Type != i2[k].Type {
			return false
		}
		if i1[k].Value != i2[k].Value {
			return false
		}
	}
	return true
}

func equal(i1, i2 []Token) bool {
	if len(i1) != len(i2) {
		return false
	}
	for k := range i1 {
		if i1[k].Type != i2[k].Type {
			return false
		}
		if i1[k].Value != i2[k].Value {
			return false
		}
		if i1[k].Pos != i2[k].Pos {
			return false
		}
		if i1[k].End != i2[k].End {
			return false
		}
		if i1[k].Line != i2[k].Line {
			return false
		}
	}
	return true
}

// runLexTest runs a single lex test case with the given delimiters.
func runLexTest(t *testing.T, test lexTest, left, right string) {
	t.Helper()
	items := collect(&test, left, right)
	if !equalSkipPosCheck(items, test.items) {
		t.Errorf("%s: got\n\t%+v\nexpected\n\t%v", test.name, items, test.items)
	}
}

func TestLex(t *testing.T) {
	for _, test := range []lexTest{
		{name: "empty", input: "", items: []Token{tEOF}},
		{name: "spaces", input: " \t\n", items: []Token{{Type: TokenTypeText, Value: " \t\n"}, tEOF}},
		{name: "text", input: `now is the time`, items: []Token{{Type: TokenTypeText, Value: "now is the time"}, tEOF}},
		{name: "text with comment", input: "hello-{{/* this is a comment */}}-world", items: []Token{
			{Type: TokenTypeText, Value: "hello-"},
			{Type: TokenTypeComment, Value: "/* this is a comment */"},
			{Type: TokenTypeText, Value: "-world"},
			tEOF,
		}},
		{name: "punctuation", input: "{{,@% }}", items: []Token{
			tLeft,
			{Type: TokenTypeChar, Value: ","},
			{Type: TokenTypeChar, Value: "@"},
			{Type: TokenTypeChar, Value: "%"},
			tSpace,
			tRight,
			tEOF,
		}},
		{name: "parens", input: "{{((3))}}", items: []Token{
			tLeft,
			tLpar,
			tLpar,
			{Type: TokenTypeNumber, Value: "3"},
			tRpar,
			tRpar,
			tRight,
			tEOF,
		}},
		{name: "empty action", input: `{{}}`, items: []Token{tLeft, tRight, tEOF}},
		{name: "for", input: `{{for}}`, items: []Token{tLeft, tFor, tRight, tEOF}},
		{name: "block", input: `{{block "foo" .}}`, items: []Token{
			tLeft, tBlock, tSpace, {Type: TokenTypeString, Value: `"foo"`}, tSpace, tDot, tRight, tEOF,
		}},
		{name: "quote", input: `{{"abc \n\t\" "}}`, items: []Token{tLeft, tQuote, tRight, tEOF}},
		{name: "raw quote", input: "{{" + raw + "}}", items: []Token{tLeft, tRawQuote, tRight, tEOF}},
		{name: "raw quote with newline", input: "{{" + rawNL + "}}", items: []Token{tLeft, tRawQuoteNL, tRight, tEOF}},
		{name: "numbers", input: "{{1 02 0x14 0X14 -7.2i 1e3 1E3 +1.2e-4 4.2i 1+2i 1_2 0x1.e_fp4 0X1.E_FP4}}", items: []Token{
			tLeft,
			{Type: TokenTypeNumber, Value: "1"},
			tSpace,
			{Type: TokenTypeNumber, Value: "02"},
			tSpace,
			{Type: TokenTypeNumber, Value: "0x14"},
			tSpace,
			{Type: TokenTypeNumber, Value: "0X14"},
			tSpace,
			{Type: TokenTypeNumber, Value: "-7.2i"},
			tSpace,
			{Type: TokenTypeNumber, Value: "1e3"},
			tSpace,
			{Type: TokenTypeNumber, Value: "1E3"},
			tSpace,
			{Type: TokenTypeNumber, Value: "+1.2e-4"},
			tSpace,
			{Type: TokenTypeNumber, Value: "4.2i"},
			tSpace,
			{Type: TokenTypeComplex, Value: "1+2i"},
			tSpace,
			{Type: TokenTypeNumber, Value: "1_2"},
			tSpace,
			{Type: TokenTypeNumber, Value: "0x1.e_fp4"},
			tSpace,
			{Type: TokenTypeNumber, Value: "0X1.E_FP4"},
			tRight,
			tEOF,
		}},
		{name: "characters", input: `{{'a' '\n' '\'' '\\' '\u00FF' '\xFF' '本'}}`, items: []Token{
			tLeft,
			{Type: TokenTypeCharConstant, Value: `'a'`},
			tSpace,
			{Type: TokenTypeCharConstant, Value: `'\n'`},
			tSpace,
			{Type: TokenTypeCharConstant, Value: `'\''`},
			tSpace,
			{Type: TokenTypeCharConstant, Value: `'\\'`},
			tSpace,
			{Type: TokenTypeCharConstant, Value: `'\u00FF'`},
			tSpace,
			{Type: TokenTypeCharConstant, Value: `'\xFF'`},
			tSpace,
			{Type: TokenTypeCharConstant, Value: `'本'`},
			tRight,
			tEOF,
		}},
		{name: "bools", input: "{{true false}}", items: []Token{
			tLeft,
			{Type: TokenTypeBool, Value: "true"},
			tSpace,
			{Type: TokenTypeBool, Value: "false"},
			tRight,
			tEOF,
		}},
		{name: "dot", input: "{{.}}", items: []Token{
			tLeft,
			tDot,
			tRight,
			tEOF,
		}},
		{name: "nil", input: "{{nil}}", items: []Token{
			tLeft,
			{Type: TokenTypeNil, Value: "nil"},
			tRight,
			tEOF,
		}},
		{name: "dots", input: "{{.x . .2 .x.y.z}}", items: []Token{
			tLeft,
			{Type: TokenTypeField, Value: ".x"},
			tSpace,
			tDot,
			tSpace,
			{Type: TokenTypeNumber, Value: ".2"},
			tSpace,
			{Type: TokenTypeField, Value: ".x"},
			{Type: TokenTypeField, Value: ".y"},
			{Type: TokenTypeField, Value: ".z"},
			tRight,
			tEOF,
		}},
		{name: "keywords", input: "{{range if else end with}}", items: []Token{
			tLeft,
			{Type: TokenTypeRange, Value: "range"},
			tSpace,
			{Type: TokenTypeIf, Value: "if"},
			tSpace,
			{Type: TokenTypeElse, Value: "else"},
			tSpace,
			{Type: TokenTypeEnd, Value: "end"},
			tSpace,
			{Type: TokenTypeWith, Value: "with"},
			tRight,
			tEOF,
		}},
		{name: "variables", input: "{{$c := printf $ $hello $23 $ $var.Field .Method}}", items: []Token{
			tLeft,
			{Type: TokenTypeVariable, Value: "$c"},
			tSpace,
			{Type: TokenTypeDeclare, Value: ":="},
			tSpace,
			{Type: TokenTypeIdentifier, Value: "printf"},
			tSpace,
			{Type: TokenTypeVariable, Value: "$"},
			tSpace,
			{Type: TokenTypeVariable, Value: "$hello"},
			tSpace,
			{Type: TokenTypeVariable, Value: "$23"},
			tSpace,
			{Type: TokenTypeVariable, Value: "$"},
			tSpace,
			{Type: TokenTypeVariable, Value: "$var"},
			{Type: TokenTypeField, Value: ".Field"},
			tSpace,
			{Type: TokenTypeField, Value: ".Method"},
			tRight,
			tEOF,
		}},
		{name: "variable invocation", input: "{{$x 23}}", items: []Token{
			tLeft,
			{Type: TokenTypeVariable, Value: "$x"},
			tSpace,
			{Type: TokenTypeNumber, Value: "23"},
			tRight,
			tEOF,
		}},
		{name: "pipeline", input: `intro {{echo hi 1.2 |noargs|args 1 "hi"}} outro`, items: []Token{
			{Type: TokenTypeText, Value: "intro "},
			tLeft,
			{Type: TokenTypeIdentifier, Value: "echo"},
			tSpace,
			{Type: TokenTypeIdentifier, Value: "hi"},
			tSpace,
			{Type: TokenTypeNumber, Value: "1.2"},
			tSpace,
			tPipe,
			{Type: TokenTypeIdentifier, Value: "noargs"},
			tPipe,
			{Type: TokenTypeIdentifier, Value: "args"},
			tSpace,
			{Type: TokenTypeNumber, Value: "1"},
			tSpace,
			{Type: TokenTypeString, Value: `"hi"`},
			tRight,
			{Type: TokenTypeText, Value: " outro"},
			tEOF,
		}},
		{name: "declaration", input: "{{$v := 3}}", items: []Token{
			tLeft,
			{Type: TokenTypeVariable, Value: "$v"},
			tSpace,
			{Type: TokenTypeDeclare, Value: ":="},
			tSpace,
			{Type: TokenTypeNumber, Value: "3"},
			tRight,
			tEOF,
		}},
		{name: "2 declarations", input: "{{$v , $w := 3}}", items: []Token{
			tLeft,
			{Type: TokenTypeVariable, Value: "$v"},
			tSpace,
			{Type: TokenTypeChar, Value: ","},
			tSpace,
			{Type: TokenTypeVariable, Value: "$w"},
			tSpace,
			{Type: TokenTypeDeclare, Value: ":="},
			tSpace,
			{Type: TokenTypeNumber, Value: "3"},
			tRight,
			tEOF,
		}},
		{name: "field of parenthesized expression", input: "{{(.X).Y}}", items: []Token{
			tLeft,
			tLpar,
			{Type: TokenTypeField, Value: ".X"},
			tRpar,
			{Type: TokenTypeField, Value: ".Y"},
			tRight,
			tEOF,
		}},
		{name: "trimming spaces before and after", input: "hello- {{- 3 -}} -world", items: []Token{
			{Type: TokenTypeText, Value: "hello- "},
			tLeft,
			{Type: TokenTypeNumber, Value: "3"},
			tRight,
			{Type: TokenTypeText, Value: " -world"},
			tEOF,
		}},
		{name: "trimming spaces before and after comment", input: "hello- {{- /* hello */ -}} -world", items: []Token{
			{Type: TokenTypeText, Value: "hello- "},
			{Type: TokenTypeComment, Value: "/* hello */"},
			{Type: TokenTypeText, Value: " -world"},
			tEOF,
		}},
		// errors
		{name: "badchar", input: "#{{\x01}}", items: []Token{
			{Type: TokenTypeText, Value: "#"},
			tLeft,
			{Type: TokenTypeError, Value: "unrecognized character in action: U+0001"},
		}},
		{name: "unclosed action", input: "{{", items: []Token{
			tLeft,
			{Type: TokenTypeError, Value: "unclosed action"},
		}},
		{name: "EOF in action", input: "{{range", items: []Token{
			tLeft,
			tRange,
			{Type: TokenTypeError, Value: "unclosed action"},
		}},
		{name: "unclosed quote", input: "{{\"\n\"}}", items: []Token{
			tLeft,
			{Type: TokenTypeError, Value: "unterminated quoted string"},
		}},
		{name: "unclosed raw quote", input: "{{`xx}}", items: []Token{
			tLeft,
			{Type: TokenTypeError, Value: "unterminated raw quoted string"},
		}},
		{name: "unclosed char constant", input: "{{'\n}}", items: []Token{
			tLeft,
			{Type: TokenTypeError, Value: "unterminated character constant"},
		}},
		{name: "bad number", input: "{{3k}}", items: []Token{
			tLeft,
			{Type: TokenTypeError, Value: `bad number syntax: "3k"`},
		}},
		{name: "unclosed paren", input: "{{(3}}", items: []Token{
			tLeft,
			tLpar,
			{Type: TokenTypeNumber, Value: "3"},
			{Type: TokenTypeError, Value: `unclosed left paren`},
		}},
		{name: "extra right paren", input: "{{3)}}", items: []Token{
			tLeft,
			{Type: TokenTypeNumber, Value: "3"},
			{Type: TokenTypeError, Value: "unexpected right paren"},
		}},

		// Fixed bugs
		// Many elements in an action blew the lookahead until
		// we made scanInsideAction not loop.
		{name: "long pipeline deadlock", input: "{{|||||}}", items: []Token{
			tLeft,
			tPipe,
			tPipe,
			tPipe,
			tPipe,
			tPipe,
			tRight,
			tEOF,
		}},
		{name: "text with bad comment", input: "hello-{{/*/}}-world", items: []Token{
			{Type: TokenTypeText, Value: "hello-"},
			{Type: TokenTypeError, Value: `unclosed comment`},
		}},
		{name: "text with comment close separated from delim", input: "hello-{{/* */ }}-world", items: []Token{
			{Type: TokenTypeText, Value: "hello-"},
			{Type: TokenTypeError, Value: `comment ends before closing delimiter`},
		}},
		// This one is an error that we can't catch because it breaks templates with
		// minimized JavaScript. Should have fixed it before Go 1.1.
		{name: "unmatched right delimiter", input: "hello-{.}}-world", items: []Token{
			{Type: TokenTypeText, Value: "hello-{.}}-world"},
			tEOF,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runLexTest(t, test, "", "")
		})
	}
}

// Some easy cases from above, but with delimiters $$ and @@
var lexDelimTests = []lexTest{
	{name: "punctuation", input: "$$,@%{{}}@@", items: []Token{
		tLeftDelim,
		{Type: TokenTypeChar, Value: ","},
		{Type: TokenTypeChar, Value: "@"},
		{Type: TokenTypeChar, Value: "%"},
		{Type: TokenTypeChar, Value: "{"},
		{Type: TokenTypeChar, Value: "{"},
		{Type: TokenTypeChar, Value: "}"},
		{Type: TokenTypeChar, Value: "}"},
		tRightDelim,
		tEOF,
	}},
	{name: "empty action", input: `$$@@`, items: []Token{tLeftDelim, tRightDelim, tEOF}},
	{name: "for", input: `$$for@@`, items: []Token{tLeftDelim, tFor, tRightDelim, tEOF}},
	{name: "quote", input: `$$"abc \n\t\" "@@`, items: []Token{tLeftDelim, tQuote, tRightDelim, tEOF}},
	{name: "raw quote", input: "$$" + raw + "@@", items: []Token{tLeftDelim, tRawQuote, tRightDelim, tEOF}},
}

var (
	tLeftDelim  = Token{Type: TokenTypeLeftDelim, Value: "$$"}
	tRightDelim = Token{Type: TokenTypeRightDelim, Value: "@@"}
)

func TestDelims(t *testing.T) {
	for _, test := range lexDelimTests {
		t.Run(test.name, func(t *testing.T) {
			runLexTest(t, test, "$$", "@@")
		})
	}
}

func TestDelimsAlphaNumeric(t *testing.T) {
	test := lexTest{name: "right delimiter with alphanumeric start", input: "{{hub .host hub}}", items: []Token{
		{Type: TokenTypeLeftDelim, Value: "{{hub"},
		{Type: TokenTypeSpace, Value: " "},
		{Type: TokenTypeField, Value: ".host"},
		{Type: TokenTypeSpace, Value: " "},
		{Type: TokenTypeRightDelim, Value: "hub}}"},
		tEOF,
	}}
	items := collect(&test, "{{hub", "hub}}")

	if !equalSkipPosCheck(items, test.items) {
		t.Errorf("%s: got\n\t%v\nexpected\n\t%v", test.name, items, test.items)
	}
}

func TestDelimsAndMarkers(t *testing.T) {
	test := lexTest{name: "delims that look like markers", input: "{{- .x -}} {{- - .x - -}}", items: []Token{
		{Type: TokenTypeLeftDelim, Value: "{{- "},
		{Type: TokenTypeField, Value: ".x"},
		{Type: TokenTypeRightDelim, Value: " -}}"},
		{Type: TokenTypeText, Value: " "},
		{Type: TokenTypeLeftDelim, Value: "{{- "},
		{Type: TokenTypeField, Value: ".x"},
		{Type: TokenTypeRightDelim, Value: " -}}"},
		tEOF,
	}}
	items := collect(&test, "{{- ", " -}}")

	if !equalSkipPosCheck(items, test.items) {
		t.Errorf("%s: got\n\t%v\nexpected\n\t%v", test.name, items, test.items)
	}
}

var lexPosTests = []lexTest{
	{name: "empty", input: "", items: []Token{{Type: TokenTypeEOF, Pos: 0, End: 0, Value: "", Line: 1}}},
	{name: "punctuation", input: "{{,@%#}}", items: []Token{
		{Type: TokenTypeLeftDelim, Pos: 0, End: 2, Value: "{{", Line: 1},
		{Type: TokenTypeChar, Pos: 2, End: 3, Value: ",", Line: 1},
		{Type: TokenTypeChar, Pos: 3, End: 4, Value: "@", Line: 1},
		{Type: TokenTypeChar, Pos: 4, End: 5, Value: "%", Line: 1},
		{Type: TokenTypeChar, Pos: 5, End: 6, Value: "#", Line: 1},
		{Type: TokenTypeRightDelim, Pos: 6, End: 8, Value: "}}", Line: 1},
		{Type: TokenTypeEOF, Pos: 8, End: 8, Value: "", Line: 1},
	}},
	{name: "sample", input: "0123{{hello}}xyz", items: []Token{
		{Type: TokenTypeText, Pos: 0, End: 4, Value: "0123", Line: 1},
		{Type: TokenTypeLeftDelim, Pos: 4, End: 6, Value: "{{", Line: 1},
		{Type: TokenTypeIdentifier, Pos: 6, End: 11, Value: "hello", Line: 1},
		{Type: TokenTypeRightDelim, Pos: 11, End: 13, Value: "}}", Line: 1},
		{Type: TokenTypeText, Pos: 13, End: 16, Value: "xyz", Line: 1},
		{Type: TokenTypeEOF, Pos: 16, End: 16, Value: "", Line: 1},
	}},
	{name: "trimafter", input: "{{x -}}\n{{y}}", items: []Token{
		{Type: TokenTypeLeftDelim, Pos: 0, End: 2, Value: "{{", Line: 1},
		{Type: TokenTypeIdentifier, Pos: 2, End: 3, Value: "x", Line: 1},
		{Type: TokenTypeRightDelim, Pos: 5, End: 7, Value: "}}", Line: 1},
		{Type: TokenTypeText, Pos: 7, End: 8, Value: "\n", Line: 1},
		{Type: TokenTypeLeftDelim, Pos: 8, End: 10, Value: "{{", Line: 2},
		{Type: TokenTypeIdentifier, Pos: 10, End: 11, Value: "y", Line: 2},
		{Type: TokenTypeRightDelim, Pos: 11, End: 13, Value: "}}", Line: 2},
		{Type: TokenTypeEOF, Pos: 13, End: 13, Value: "", Line: 2},
	}},
	{name: "trimbefore", input: "{{x}}\n{{- y}}", items: []Token{
		{Type: TokenTypeLeftDelim, Pos: 0, End: 2, Value: "{{", Line: 1},
		{Type: TokenTypeIdentifier, Pos: 2, End: 3, Value: "x", Line: 1},
		{Type: TokenTypeRightDelim, Pos: 3, End: 5, Value: "}}", Line: 1},
		{Type: TokenTypeText, Pos: 5, End: 6, Value: "\n", Line: 1},
		{Type: TokenTypeLeftDelim, Pos: 6, End: 8, Value: "{{", Line: 2},
		{Type: TokenTypeIdentifier, Pos: 10, End: 11, Value: "y", Line: 2},
		{Type: TokenTypeRightDelim, Pos: 11, End: 13, Value: "}}", Line: 2},
		{Type: TokenTypeEOF, Pos: 13, End: 13, Value: "", Line: 2},
	}},
	{name: "longcomment", input: "{{/*\n*/}}\n{{undefinedFunction \"test\"}}", items: []Token{
		{Type: TokenTypeComment, Pos: 2, End: 7, Value: "/*\n*/", Line: 1},
		{Type: TokenTypeText, Pos: 9, End: 10, Value: "\n", Line: 2},
		{Type: TokenTypeLeftDelim, Pos: 10, End: 12, Value: "{{", Line: 3},
		{Type: TokenTypeIdentifier, Pos: 12, End: 29, Value: "undefinedFunction", Line: 3},
		{Type: TokenTypeSpace, Pos: 29, End: 30, Value: " ", Line: 3},
		{Type: TokenTypeString, Pos: 30, End: 36, Value: "\"test\"", Line: 3},
		{Type: TokenTypeRightDelim, Pos: 36, End: 38, Value: "}}", Line: 3},
		{Type: TokenTypeEOF, Pos: 38, End: 38, Value: "", Line: 3},
	}},
}

// The other tests don't check position, to make the test cases easier to construct.
// This one does.
func TestPos(t *testing.T) {
	for _, test := range lexPosTests {
		items := collect(&test, "", "")
		if !equal(items, test.items) {
			t.Errorf("%s: got\n\t%v\nexpected\n\t%v", test.name, items, test.items)
			if len(items) == len(test.items) {
				// Detailed print; avoid item.String() to expose the position value.
				for i := range items {
					if !equal(items[i:i+1], test.items[i:i+1]) {
						i1 := items[i]
						i2 := test.items[i]
						t.Errorf("\t#%d: got {%v %d %d %q %d} expected {%v %d %d %q %d}",
							i, i1.Type, i1.Pos, i1.End, i1.Value, i1.Line, i2.Type, i2.Pos, i2.End, i2.Value, i2.Line)
					}
				}
			}
		}
	}
}
