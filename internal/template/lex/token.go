package lex

import "fmt"

// Token represents lexical unit
type Token struct {
	Type      TokenType
	Pos       Pos // The starting position, in bytes, of this item in the input string.
	End       Pos // The end position (exclusive), in bytes, of this item in the input string.
	Value     string
	Line      int
	TrimLeft  bool // {{- was used
	TrimRight bool // -}} was used
}

func (i Token) String() string {
	switch {
	case i.Type == TokenTypeEOF:
		return "EOF"
	case i.Type == TokenTypeError:
		return i.Value
	case i.Type > TokenTypeKeyword:
		return fmt.Sprintf("<%s>", i.Value)
	case len(i.Value) > 10:
		return fmt.Sprintf("%.10q...", i.Value)
	}
	return fmt.Sprintf("%q", i.Value)
}
