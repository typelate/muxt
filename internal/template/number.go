package template

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/template/lex"
)

// NumberNode holds a number: signed or unsigned integer, float, or complex.
// The value is parsed and stored under all the types that can represent the value.
// This simulates in a small amount of code the behavior of Go's ideal constants.
type NumberNode struct {
	tr         *Tree
	start, end lex.Pos

	IsInt      bool       // Number has an integral value.
	IsUint     bool       // Number has an unsigned integral value.
	IsFloat    bool       // Number has a floating-point value.
	IsComplex  bool       // Number is complex.
	Int64      int64      // The signed integer value.
	Uint64     uint64     // The unsigned integer value.
	Float64    float64    // The floating-point value.
	Complex128 complex128 // The complex value.
	Text       string     // The original textual representation from the input.
}

func (t *Tree) newNumber(start, end lex.Pos, text string, typ lex.TokenType) (*NumberNode, error) {
	n := &NumberNode{tr: t, start: start, end: end, Text: text}
	switch typ {
	case lex.TokenTypeCharConstant:
		rn, _, tail, err := strconv.UnquoteChar(text[1:], text[0])
		if err != nil {
			return nil, err
		}
		if tail != "'" {
			return nil, fmt.Errorf("malformed character constant: %s", text)
		}
		n.Int64 = int64(rn)
		n.IsInt = true
		n.Uint64 = uint64(rn)
		n.IsUint = true
		n.Float64 = float64(rn) // odd but those are the rules.
		n.IsFloat = true
		return n, nil
	case lex.TokenTypeComplex:
		// fmt.Sscan can parse the pair, so let it do the work.
		if _, err := fmt.Sscan(text, &n.Complex128); err != nil {
			return nil, err
		}
		n.IsComplex = true
		simplifyComplex(n)
		return n, nil
	default:
		// Imaginary constants can only be complex unless they are zero.
		if len(text) > 0 && text[len(text)-1] == 'i' {
			f, err := strconv.ParseFloat(text[:len(text)-1], 64)
			if err == nil {
				n.IsComplex = true
				n.Complex128 = complex(0, f)
				simplifyComplex(n)
				return n, nil
			}
		}
		// Do integer test first so we get 0x123 etc.
		u, err := strconv.ParseUint(text, 0, 64) // will fail for -0; fixed below.
		if err == nil {
			n.IsUint = true
			n.Uint64 = u
		}
		i, err := strconv.ParseInt(text, 0, 64)
		if err == nil {
			n.IsInt = true
			n.Int64 = i
			if i == 0 {
				n.IsUint = true // in case of -0.
				n.Uint64 = u
			}
		}
		// If an integer extraction succeeded, promote the float.
		if n.IsInt {
			n.IsFloat = true
			n.Float64 = float64(n.Int64)
		} else if n.IsUint {
			n.IsFloat = true
			n.Float64 = float64(n.Uint64)
		} else {
			f, err := strconv.ParseFloat(text, 64)
			if err == nil {
				// If we parsed it as a float, but it looks like an integer,
				// it's a huge number too large to fit in an int. Reject it.
				if !strings.ContainsAny(text, ".eEpP") {
					return nil, fmt.Errorf("integer overflow: %q", text)
				}
				n.IsFloat = true
				n.Float64 = f
				// If a floating-point extraction succeeded, extract the int if needed.
				if !n.IsInt && float64(int64(f)) == f {
					n.IsInt = true
					n.Int64 = int64(f)
				}
				if !n.IsUint && float64(uint64(f)) == f {
					n.IsUint = true
					n.Uint64 = uint64(f)
				}
			}
		}
		if !n.IsInt && !n.IsUint && !n.IsFloat {
			return nil, fmt.Errorf("illegal number syntax: %q", text)
		}
		return n, nil
	}
}

// simplifyComplex pulls out any other types that are represented by the complex number.
// These all require that the imaginary part be zero.
func simplifyComplex(n *NumberNode) {
	n.IsFloat = imag(n.Complex128) == 0
	if n.IsFloat {
		n.Float64 = real(n.Complex128)
		n.IsInt = float64(int64(n.Float64)) == n.Float64
		if n.IsInt {
			n.Int64 = int64(n.Float64)
		}
		n.IsUint = float64(uint64(n.Float64)) == n.Float64
		if n.IsUint {
			n.Uint64 = uint64(n.Float64)
		}
	}
}

func (n NumberNode) End() lex.Pos                { return n.end }
func (n NumberNode) Start() lex.Pos              { return n.start }
func (n NumberNode) String() string              { return n.tr.src(n.Start(), n.End()) }
func (n NumberNode) writeTo(sb *strings.Builder) { sb.WriteString(n.String()) }
func (n NumberNode) tree() *Tree                 { return n.tr }
