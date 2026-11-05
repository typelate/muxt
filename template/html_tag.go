package template

import "strings"

// tagPart is where in a tag an action stands.
type tagPart uint8

const (
	tagNone        tagPart = iota // not in a tag
	tagStart                      // in a tag, where an attribute name may start
	tagAttrName                   // in an attribute name
	tagAfterName                  // after an attribute name, before a '='
	tagBeforeValue                // after a '=', before the value
	tagValue                      // in an attribute value
	tagUnknown                    // in markup the scan cannot follow
)

// tagScan follows the template's own text through a tag the parser left
// as text, such as <p {{.Name}}=v> or <p data-{{.Name}}=v>, as
// html/template's escaper follows every tag. The engine writes such a
// tag as text, so an action in it must be escaped for where it stands:
// as an attribute name, or as an attribute value between the value's
// quotes.
type tagScan struct {
	part    tagPart
	element string // the tag's name, lowercased
	attr    string // the attribute name's leading text, lowercased; "" when an action starts the name
	quote   byte   // for tagValue: '"', '\'', or 0 for an unquoted value
}

const tagSpace = " \t\n\f\r"

// text returns the scan after the template text s.
func (t tagScan) text(s string) tagScan {
	for len(s) > 0 && t.part != tagUnknown {
		t, s = t.step(s)
	}
	return t
}

// step consumes the start of s, which is not empty, and returns the scan
// after it with the rest of s. It follows html/template's transitions
// for text, a tag, an attribute name and an attribute value.
func (t tagScan) step(s string) (tagScan, string) {
	switch t.part {
	case tagNone:
		for {
			i := strings.IndexByte(s, '<')
			if i < 0 || i+1 == len(s) {
				return t, ""
			}
			s = s[i+1:]
			name := s
			if name[0] == '/' {
				name = name[1:]
			}
			if n := tagNameLen(name); n > 0 {
				return tagScan{part: tagStart, element: strings.ToLower(name[:n])}, name[n:]
			}
		}
	case tagStart:
		s = strings.TrimLeft(s, tagSpace)
		if s == "" {
			return t, ""
		}
		if s[0] == '>' {
			if isRawTextElement(t.element) {
				// What follows is script or style, not HTML text.
				return tagScan{part: tagUnknown}, ""
			}
			return tagScan{}, s[1:]
		}
		n, ok := attrNameLen(s)
		if !ok || n == 0 {
			return tagScan{part: tagUnknown}, ""
		}
		t.attr = strings.ToLower(s[:n])
		t.part = tagAfterName
		if n == len(s) {
			t.part = tagAttrName
		}
		return t, s[n:]
	case tagAttrName:
		n, ok := attrNameLen(s)
		if !ok {
			return tagScan{part: tagUnknown}, ""
		}
		if n < len(s) {
			t.part = tagAfterName
		}
		return t, s[n:]
	case tagAfterName:
		s = strings.TrimLeft(s, tagSpace)
		if s == "" {
			return t, ""
		}
		if s[0] != '=' {
			// A valueless attribute: the tag's end or the next name.
			return tagScan{part: tagStart, element: t.element}, s
		}
		t.part = tagBeforeValue
		return t, s[1:]
	case tagBeforeValue:
		s = strings.TrimLeft(s, tagSpace)
		if s == "" {
			return t, ""
		}
		t.part, t.quote = tagValue, 0
		if s[0] == '"' || s[0] == '\'' {
			t.quote, s = s[0], s[1:]
		}
		return t, s
	case tagValue:
		after := tagScan{part: tagStart, element: t.element}
		if t.quote != 0 {
			i := strings.IndexByte(s, t.quote)
			if i < 0 {
				return t, ""
			}
			return after, s[i+1:]
		}
		i := strings.IndexAny(s, tagSpace+">")
		if i < 0 {
			i = len(s)
		}
		if strings.ContainsAny(s[:i], "\"'<=`") {
			return tagScan{part: tagUnknown}, ""
		}
		if i == len(s) {
			return t, ""
		}
		return after, s[i:]
	}
	return tagScan{part: tagUnknown}, ""
}

// action returns the scan where an action writes its output, and the
// scan after it: an action where a name may start, or after one, writes
// a name, and an action after a '=' writes an unquoted value.
func (t tagScan) action() (at, after tagScan) {
	switch t.part {
	case tagStart, tagAfterName:
		t.part, t.attr = tagAttrName, ""
	case tagBeforeValue:
		t.part, t.quote = tagValue, 0
	}
	return t, t
}

// join returns the scan after a control structure whose branches end in
// a and b, tagUnknown when they end in different places.
func (a tagScan) join(b tagScan) tagScan {
	if a == b {
		return a
	}
	a, _ = a.action()
	b, _ = b.action()
	if a == b {
		return a
	}
	return tagScan{part: tagUnknown}
}

// context is the HTMLContext of an action or template call at t, which
// is in a tag.
func (t tagScan) context() *HTMLContext {
	ctx := &HTMLContext{State: "tag", Element: t.element, tag: t.part}
	if t.part == tagValue {
		ctx.Attr, ctx.quote = t.attr, t.quote
	}
	return ctx
}

// tagNameLen is the length of the tag name s starts with: letters and
// digits after a letter, with single ':' and '-' between them.
func tagNameLen(s string) int {
	if s == "" || !isASCIILetter(s[0]) {
		return 0
	}
	alnum := func(c byte) bool { return isASCIILetter(c) || isASCIIDigit(c) }
	n := 1
	for n < len(s) {
		switch c := s[n]; {
		case alnum(c):
			n++
		case (c == ':' || c == '-') && n+1 < len(s) && alnum(s[n+1]):
			n += 2
		default:
			return n
		}
	}
	return n
}

// attrNameLen is the length of the attribute name text s starts with.
// It reports false for a character no attribute name holds.
func attrNameLen(s string) (int, bool) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\f', '\r', '=', '>':
			return i, true
		case '\'', '"', '<':
			return 0, false
		}
	}
	return len(s), true
}

func isASCIILetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

func isASCIIDigit(c byte) bool { return '0' <= c && c <= '9' }

// filterFailsafe is what html/template writes in place of a value that
// is unsafe where it stands.
const filterFailsafe = "ZgotmplZ"

// htmlNameFilter is html/template's filter for an action that writes an
// attribute name: the lowercased name when it is letters and digits and
// names an attribute with a plain text value, ZgotmplZ otherwise, so
// that data cannot name an event handler or a URL attribute, or end the
// name. A value trusted as an attribute is written as it is.
func htmlNameFilter(s string, trust Trust) string {
	if trust == TrustHTMLAttr {
		return s
	}
	if s == "" {
		return filterFailsafe
	}
	s = strings.ToLower(s)
	if !plainAttr(s) {
		return filterFailsafe
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !isASCIIDigit(c) && !('a' <= c && c <= 'z') {
			return filterFailsafe
		}
	}
	return s
}

// plainAttr reports whether html/template takes the value of the named
// attribute, which is lowercase, for plain text: not a URL, script,
// style or srcset, and not a value it calls unsafe to set from data.
func plainAttr(name string) bool {
	switch name {
	case "accept-charset", "action", "archive", "async", "background", "cite",
		"challenge", "charset", "classid", "codebase", "content", "crossorigin",
		"data", "defer", "enctype", "form", "formaction", "formenctype",
		"formmethod", "formnovalidate", "href", "http-equiv", "icon", "keytype",
		"language", "longdesc", "manifest", "method", "novalidate", "pattern",
		"poster", "profile", "rel", "sandbox", "src", "srcdoc", "srcset",
		"style", "type", "usemap", "value", "xmlns":
		return false
	}
	if isURLAttr(name) || isSrcsetAttr(name) || isJSEventAttr(name) || strings.HasPrefix(name, "on") {
		return false
	}
	return !strings.Contains(name, "src") && !strings.Contains(name, "uri") && !strings.Contains(name, "url")
}
