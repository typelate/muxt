package template

import (
	"fmt"
	"testing"

	"github.com/typelate/muxt/internal/template/lex"
)

// numberCase describes a single numeric literal and the classifications
// newNumber should assign to it. A zero-valued want* field with its matching
// is* flag false means "no value expected in that kind".
type numberCase struct {
	text string

	isInt, isUint, isFloat, isComplex bool

	wantInt     int64
	wantUint    uint64
	wantFloat   float64
	wantComplex complex128
}

// parseErr is the sentinel for cases that must fail to parse. A case with all
// four is* flags false is treated as a parse-error case.
func (c numberCase) parseErr() bool {
	return !c.isInt && !c.isUint && !c.isFloat && !c.isComplex
}

func TestNumberParse(t *testing.T) {
	cases := []numberCase{
		// --- basics ---
		{text: "0", isInt: true, isUint: true, isFloat: true},
		{text: "-0", isInt: true, isUint: true, isFloat: true}, // -0 is still a uint
		{text: "73", isInt: true, isUint: true, isFloat: true, wantInt: 73, wantUint: 73, wantFloat: 73},
		{text: "7_3", isInt: true, isUint: true, isFloat: true, wantInt: 73, wantUint: 73, wantFloat: 73},
		{text: "0b10_010_01", isInt: true, isUint: true, isFloat: true, wantInt: 73, wantUint: 73, wantFloat: 73},
		{text: "0B10_010_01", isInt: true, isUint: true, isFloat: true, wantInt: 73, wantUint: 73, wantFloat: 73},
		{text: "073", isInt: true, isUint: true, isFloat: true, wantInt: 0o73, wantUint: 0o73, wantFloat: 0o73},
		{text: "0o73", isInt: true, isUint: true, isFloat: true, wantInt: 0o73, wantUint: 0o73, wantFloat: 0o73},
		{text: "0O73", isInt: true, isUint: true, isFloat: true, wantInt: 0o73, wantUint: 0o73, wantFloat: 0o73},
		{text: "0x73", isInt: true, isUint: true, isFloat: true, wantInt: 0x73, wantUint: 0x73, wantFloat: 0x73},
		{text: "0X73", isInt: true, isUint: true, isFloat: true, wantInt: 0x73, wantUint: 0x73, wantFloat: 0x73},
		{text: "0x7_3", isInt: true, isUint: true, isFloat: true, wantInt: 0x73, wantUint: 0x73, wantFloat: 0x73},
		{text: "-73", isInt: true, isFloat: true, wantInt: -73, wantFloat: -73},
		{text: "+73", isInt: true, isFloat: true, wantInt: 73, wantFloat: 73},
		{text: "100", isInt: true, isUint: true, isFloat: true, wantInt: 100, wantUint: 100, wantFloat: 100},
		{text: "1e9", isInt: true, isUint: true, isFloat: true, wantInt: 1e9, wantUint: 1e9, wantFloat: 1e9},
		{text: "-1e9", isInt: true, isFloat: true, wantInt: -1e9, wantFloat: -1e9},
		{text: "-1.2", isFloat: true, wantFloat: -1.2},
		{text: "1e19", isUint: true, isFloat: true, wantUint: 1e19, wantFloat: 1e19},
		{text: "1e1_9", isUint: true, isFloat: true, wantUint: 1e19, wantFloat: 1e19},
		{text: "1E19", isUint: true, isFloat: true, wantUint: 1e19, wantFloat: 1e19},
		{text: "-1e19", isFloat: true, wantFloat: -1e19},
		{text: "0x_1p4", isInt: true, isUint: true, isFloat: true, wantInt: 16, wantUint: 16, wantFloat: 16},
		{text: "0X_1P4", isInt: true, isUint: true, isFloat: true, wantInt: 16, wantUint: 16, wantFloat: 16},
		{text: "0x_1p-4", isFloat: true, wantFloat: 1. / 16.},

		// --- complex ---
		{text: "4i", isComplex: true, wantComplex: 4i},
		{text: "-1.2+4.2i", isComplex: true, wantComplex: -1.2 + 4.2i},
		{text: "073i", isComplex: true, wantComplex: 73i}, // not octal despite leading 0

		// complex values with zero imaginary collapse to float (and sometimes int)
		{text: "0i", isInt: true, isUint: true, isFloat: true, isComplex: true},
		{text: "-1.2+0i", isFloat: true, isComplex: true, wantFloat: -1.2, wantComplex: -1.2},
		{text: "-12+0i", isInt: true, isFloat: true, isComplex: true, wantInt: -12, wantFloat: -12, wantComplex: -12},
		{text: "13+0i", isInt: true, isUint: true, isFloat: true, isComplex: true, wantInt: 13, wantUint: 13, wantFloat: 13, wantComplex: 13},

		// --- funny bases ---
		{text: "0123", isInt: true, isUint: true, isFloat: true, wantInt: 0o123, wantUint: 0o123, wantFloat: 0o123},
		{text: "-0x0", isInt: true, isUint: true, isFloat: true},
		{text: "0xdeadbeef", isInt: true, isUint: true, isFloat: true, wantInt: 0xdeadbeef, wantUint: 0xdeadbeef, wantFloat: 0xdeadbeef},

		// --- character constants ---
		{text: `'a'`, isInt: true, isUint: true, isFloat: true, wantInt: 'a', wantUint: 'a', wantFloat: 'a'},
		{text: `'\n'`, isInt: true, isUint: true, isFloat: true, wantInt: '\n', wantUint: '\n', wantFloat: '\n'},
		{text: `'\\'`, isInt: true, isUint: true, isFloat: true, wantInt: '\\', wantUint: '\\', wantFloat: '\\'},
		{text: `'\''`, isInt: true, isUint: true, isFloat: true, wantInt: '\'', wantUint: '\'', wantFloat: '\''},
		{text: `'\xFF'`, isInt: true, isUint: true, isFloat: true, wantInt: 0xFF, wantUint: 0xFF, wantFloat: 0xFF},
		{text: `'パ'`, isInt: true, isUint: true, isFloat: true, wantInt: 0x30d1, wantUint: 0x30d1, wantFloat: 0x30d1},
		{text: `'\u30d1'`, isInt: true, isUint: true, isFloat: true, wantInt: 0x30d1, wantUint: 0x30d1, wantFloat: 0x30d1},
		{text: `'\U000030d1'`, isInt: true, isUint: true, isFloat: true, wantInt: 0x30d1, wantUint: 0x30d1, wantFloat: 0x30d1},

		// --- parse errors ---
		{text: "+-2"},
		{text: "0x123."},
		{text: "1e."},
		{text: "0xi."},
		{text: "1+2."},
		{text: "'x"},
		{text: "'xx'"},
		{text: "'433937734937734969526500969526500'"}, // integer too large (issue 10634)

		// regression: 0xe was once parsed as floating point (issue 8622)
		{text: "0xef", isInt: true, isUint: true, isFloat: true, wantInt: 0xef, wantUint: 0xef, wantFloat: 0xef},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			checkNumberParse(t, tc)
		})
	}
}

func checkNumberParse(t *testing.T, tc numberCase) {
	t.Helper()

	typ := classifyNumberText(tc.text)
	var tree *Tree
	n, err := tree.newNumber(0, 0, tc.text, typ)

	if tc.parseErr() {
		if err == nil {
			t.Fatalf("newNumber(%q) = no error, want error", tc.text)
		}
		if *debug {
			fmt.Printf("%s\n\t%s\n", tc.text, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("newNumber(%q) error: %s", tc.text, err)
	}

	checkKind(t, tc.text, "Int", n.IsInt, tc.isInt, n.Int64, tc.wantInt)
	checkKind(t, tc.text, "Uint", n.IsUint, tc.isUint, n.Uint64, tc.wantUint)
	checkKind(t, tc.text, "Float", n.IsFloat, tc.isFloat, n.Float64, tc.wantFloat)
	checkKind(t, tc.text, "Complex", n.IsComplex, tc.isComplex, n.Complex128, tc.wantComplex)
}

// checkKind asserts a NumberNode's kind flag matches wantFlag and, when the
// flag is set, that the kind's value matches want. Generic over the value type
// so one helper serves Int/Uint/Float/Complex without manual reflection.
func checkKind[T comparable](t *testing.T, text, kind string, gotFlag, wantFlag bool, got, want T) {
	t.Helper()
	if gotFlag != wantFlag {
		t.Errorf("Is%s(%q) = %t, want %t", kind, text, gotFlag, wantFlag)
	}
	if wantFlag && got != want {
		t.Errorf("%s(%q) = %v, want %v", kind, text, got, want)
	}
}

// classifyNumberText picks the token type newNumber expects for a given
// literal. fmt.Sscan into complex128 is the heuristic used by text/template.
func classifyNumberText(text string) lex.TokenType {
	if text[0] == '\'' {
		return lex.TokenTypeCharConstant
	}
	var c complex128
	if _, err := fmt.Sscan(text, &c); err == nil {
		return lex.TokenTypeComplex
	}
	return lex.TokenTypeNumber
}
