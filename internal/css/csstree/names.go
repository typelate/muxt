// Ported from lib/utils/names.js.

package csstree

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Upstream memoizes descriptors in two unbounded Maps keyed by every name it
// has seen; this port computes them on each call to avoid package-level state.

const hyphenMinus = '-' // '-'.charCodeAt()

// KeywordDescriptor is the result of Keyword. Field order and JSON keys
// match upstream.
type KeywordDescriptor struct {
	Basename string `json:"basename"`
	Name     string `json:"name"`
	Prefix   string `json:"prefix"`
	Vendor   string `json:"vendor"`
	Custom   bool   `json:"custom"`
}

// PropertyDescriptor is the result of Property. Field order and JSON keys
// match upstream.
type PropertyDescriptor struct {
	Basename string `json:"basename"`
	Name     string `json:"name"`
	Hack     string `json:"hack"`
	Vendor   string `json:"vendor"`
	Prefix   string `json:"prefix"`
	Custom   bool   `json:"custom"`
}

// Keyword describes an identifier: its lower-cased name split into vendor
// prefix and basename. Custom property names (--*) get no vendor prefix.
func Keyword(keyword string) KeywordDescriptor {
	return getKeywordDescriptor(keyword)
}

// Property describes a property name: a leading browser hack (one of
// _ * $ # + & / //), vendor prefix and basename. Names other than custom
// properties are lower-cased.
func Property(property string) PropertyDescriptor {
	return getPropertyDescriptor(property)
}

// VendorPrefix returns the vendor prefix (such as "-moz-") of str[offset:],
// or "" when it has none.
func VendorPrefix(str string, offset int) string {
	return getVendorPrefix(str, offset)
}

// IsCustomProperty reports whether str[offset:] starts with "--".
func IsCustomProperty(str string, offset int) bool {
	return len(str)-offset >= 2 &&
		str[offset] == hyphenMinus &&
		str[offset+1] == hyphenMinus
}

func getVendorPrefix(str string, offset int) string {
	// verdor prefix should be at least 3 chars length
	if len(str)-offset >= 3 {
		// vendor prefix starts with hyper minus following non-hyper minus
		if str[offset] == hyphenMinus &&
			str[offset+1] != hyphenMinus {
			// vendor prefix should contain a hyper minus at the ending
			secondDashIndex := strings.IndexByte(str[offset+2:], '-')

			if secondDashIndex != -1 {
				secondDashIndex += offset + 2
				return str[offset : secondDashIndex+1]
			}
		}
	}

	return ""
}

func getKeywordDescriptor(keyword string) KeywordDescriptor {
	name := toLowerCase(keyword)

	custom := IsCustomProperty(name, 0)
	vendor := ""
	if !custom {
		vendor = getVendorPrefix(name, 0)
	}
	return KeywordDescriptor{
		Basename: name[len(vendor):],
		Name:     name,
		Prefix:   vendor,
		Vendor:   vendor,
		Custom:   custom,
	}
}

func getPropertyDescriptor(property string) PropertyDescriptor {
	name := property
	hack := ""
	if len(property) > 0 {
		hack = property[:1]
	}

	if hack == "/" {
		if len(property) > 1 && property[1] == '/' {
			hack = "//"
		} else {
			hack = "/"
		}
	} else if hack != "_" &&
		hack != "*" &&
		hack != "$" &&
		hack != "#" &&
		hack != "+" &&
		hack != "&" {
		hack = ""
	}

	custom := IsCustomProperty(name, len(hack))

	if !custom {
		name = toLowerCase(name)
	}

	vendor := ""
	if !custom {
		vendor = getVendorPrefix(name, len(hack))
	}
	prefix := name[:len(hack)+len(vendor)]
	return PropertyDescriptor{
		Basename: name[len(prefix):],
		Name:     name[len(hack):],
		Hack:     hack,
		Vendor:   vendor,
		Prefix:   prefix,
		Custom:   custom,
	}
}

// toLowerCase lowercases s as JavaScript's String.prototype.toLowerCase
// does, which upstream uses for names. strings.ToLower maps each rune on its
// own; JavaScript also applies the rules of SpecialCasing.txt that do not
// depend on a language: U+0130 (İ) becomes i followed by U+0307 COMBINING
// DOT ABOVE, not i alone, and a capital sigma ending a word becomes a final
// sigma (ς).
func toLowerCase(s string) string {
	if !strings.ContainsAny(s, "\u0130\u03a3") {
		return strings.ToLower(s)
	}

	var b strings.Builder
	b.Grow(len(s) + 1)
	for i, r := range s {
		switch r {
		case '\u0130':
			b.WriteString("i\u0307")
		case '\u03a3':
			if isFinalSigma(s, i) {
				b.WriteRune('\u03c2')
			} else {
				b.WriteRune('\u03c3')
			}
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// isFinalSigma reports whether the capital sigma at s[i] is preceded by a
// cased letter and not followed by one, skipping case-ignorable runes on
// both sides. This is the Final_Sigma condition as ICU (used by V8) tests
// it: a rune that is both cased and case-ignorable is skipped.
func isFinalSigma(s string, i int) bool {
	for before := s[:i]; before != ""; {
		r, size := utf8.DecodeLastRuneInString(before)
		before = before[:len(before)-size]
		if isCaseIgnorable(r) {
			continue
		}
		if !isCased(r) {
			return false
		}
		for _, r := range s[i+len("\u03a3"):] {
			if !isCaseIgnorable(r) {
				return !isCased(r)
			}
		}
		return true
	}
	return false
}

// isCased reports whether r has the Unicode Cased property.
func isCased(r rune) bool {
	return unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

// isCaseIgnorable reports whether r has the Unicode Case_Ignorable property:
// a nonspacing, enclosing or format mark, a modifier, or a Word_Break
// MidLetter, MidNumLet or Single_Quote character.
func isCaseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '\u00b7', '\u0387', '\u055f', '\u05f4', '\u2018', '\u2019',
		'\u2024', '\u2027', '\ufe13', '\ufe52', '\ufe55', '\uff07', '\uff0e', '\uff1a':
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}
