package csstree

import (
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// The spec compares identifiers by their value, that is with escapes
// decoded (CSS Syntax 3 §4.3.11 consume a name), and ASCII
// case-insensitively (CSS Syntax 3 §8, CSS Values 4 §2.1): r\ed, \72 ed and
// RED are the keyword red. Upstream compares the source text of a token, so
// a keyword with an escape never matches unless the escape is the \0 or \9
// hack, which it drops. The helpers below keep upstream's comparisons and
// add the comparison of the decoded value.

// asciiLowerCase lowercases the ASCII letters of s only, as the spec's
// ASCII case-insensitive match does.
func asciiLowerCase(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] |= 0x20
				}
			}
			return string(b)
		}
	}
	return s
}

// isNameToken reports whether token is a token with a name: an ident,
// function or at-keyword token.
func isNameToken(token *matchToken) bool {
	switch token.typ {
	case tokenizer.Ident, tokenizer.Function, tokenizer.AtKeyword:
		return true
	}
	return false
}

// decodedNames returns the values of a name token with escapes: its source
// text decoded, and its source text without the \0 or \9 hack decoded. It
// returns nil for a token without escapes, whose value is its source text,
// and for other tokens.
func decodedNames(token *matchToken) []string {
	if !isNameToken(token) || !strings.Contains(token.value, `\`) {
		return nil
	}

	names := []string{DecodeIdent(token.value)}
	if withoutHack := keywordHack.ReplaceAllString(token.value, ""); withoutHack != token.value {
		names = append(names, DecodeIdent(withoutHack))
	}
	return names
}

// tokenNameIs reports whether the value of a name token is an ASCII
// case-insensitive match for name, a lowercase keyword, function name
// (with the parenthesis) or at-keyword (with the @), once escapes are
// decoded.
func tokenNameIs(token *matchToken, name string) bool {
	for _, decoded := range decodedNames(token) {
		if areStringsEqualCaseInsensitive(decoded, name) {
			return true
		}
	}
	return false
}

// tokenValue returns the value of token: its source text with escapes
// decoded for a name token, its source text otherwise.
func tokenValue(token *matchToken) string {
	if !isNameToken(token) || !strings.Contains(token.value, `\`) {
		return token.value
	}
	return DecodeIdent(token.value)
}

// dimensionUnit returns the unit of a dimension token whose number ends at
// numberEnd, without the IE \0 or \9 hack (123px\9) and with escapes
// decoded.
func dimensionUnit(value string, numberEnd int) string {
	unit := value[min(numberEnd, len(value)):]
	if !strings.Contains(unit, `\`) {
		return unit
	}

	// find the escape at the end of the unit, if it is the hack
	for i := 0; i < len(unit); {
		if unit[i] != '\\' {
			i++
			continue
		}
		if i == len(unit)-2 && tokenizer.IsDigit(int(unit[i+1])) {
			unit = unit[:i]
			break
		}
		if i == len(unit)-1 || !tokenizer.IsValidEscape('\\', int(unit[i+1])) {
			i++
			continue
		}
		i = tokenizer.ConsumeEscaped(unit, i)
	}

	return DecodeIdent(unit)
}

// enumLookup returns the state of the first of names in enumMap, ASCII
// case-insensitively.
func enumLookup(enumMap map[string]*matchGraphNode, names []string) (*matchGraphNode, bool) {
	for _, name := range names {
		if next, ok := enumMap[asciiLowerCase(name)]; ok {
			return next, true
		}
	}
	return nil, false
}
