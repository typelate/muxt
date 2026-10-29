package selectors

import (
	"strings"

	"github.com/typelate/muxt/internal/template"
)

// selectorList is a comma-separated group of selectors; an element matches
// when any alternative matches (logical OR in three-valued Kleene logic).
type selectorList struct {
	alternatives []matcher
}

func (l selectorList) match(n template.Node) Trit {
	result := TritNo
	for _, a := range l.alternatives {
		result = result.Or(a.match(n))
		if result == TritYes {
			return TritYes
		}
	}
	return result
}

// compoundSelector is a sequence of simple selectors that must all match the
// same element (logical AND, evaluated in three-valued Kleene logic).
type compoundSelector struct {
	simples []matcher
}

func (c compoundSelector) match(n template.Node) Trit {
	result := TritYes
	for _, s := range c.simples {
		result = result.And(s.match(n))
		if result == TritNo {
			return TritNo
		}
	}
	return result
}

// typeSelector matches an element by tag name.
type typeSelector struct {
	tag string
}

func (t typeSelector) match(n template.Node) Trit {
	el, ok := n.(*template.HTMLElementNode)
	if !ok {
		return TritNo
	}
	if strings.EqualFold(el.TagName, t.tag) {
		return TritYes
	}
	return TritNo
}

// universalSelector (*) matches any element.
type universalSelector struct{}

func (universalSelector) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); ok {
		return TritYes
	}
	return TritNo
}

// idSelector (#id) matches an element whose id attribute equals id.
type idSelector struct {
	id string
}

func (s idSelector) match(n template.Node) Trit {
	el, ok := n.(*template.HTMLElementNode)
	if !ok {
		return TritNo
	}
	attr, presence := findAttr(el, "id")
	if presence != TritYes {
		return presence
	}
	val, static := staticValue(attrValueParts(attr))
	if !static {
		return TritMaybe
	}
	if strings.TrimSpace(val) == s.id {
		return TritYes
	}
	return TritNo
}

// classSelector (.class) matches an element whose class attribute contains
// class as a whitespace-separated token.
type classSelector struct {
	class string
}

func (s classSelector) match(n template.Node) Trit {
	el, ok := n.(*template.HTMLElementNode)
	if !ok {
		return TritNo
	}
	attr, presence := findAttr(el, "class")
	if presence != TritYes {
		return presence
	}
	sawDynamic := false
	for _, tok := range classTokens(attrValueParts(attr)) {
		if tok.dynamic {
			sawDynamic = true
			continue
		}
		if tok.text == s.class {
			return TritYes
		}
	}
	if sawDynamic {
		return TritMaybe
	}
	return TritNo
}

// attrOp is an attribute selector operator.
type attrOp uint8

const (
	attrExists    attrOp = iota // [name]
	attrEquals                  // [name=value]
	attrIncludes                // [name~=value]
	attrDashMatch               // [name|=value]
	attrPrefix                  // [name^=value]
	attrSuffix                  // [name$=value]
	attrSubstring               // [name*=value]
)

// attrSelector ([name], [name=value]) matches on an element attribute.
type attrSelector struct {
	name string
	op   attrOp
	val  string
	ci   bool // case-insensitive ([name=value i])
}

func (s attrSelector) match(n template.Node) Trit {
	el, ok := n.(*template.HTMLElementNode)
	if !ok {
		return TritNo
	}
	attr, presence := findAttr(el, s.name)
	if s.op == attrExists || presence != TritYes {
		return presence
	}
	val, static := staticValue(attrValueParts(attr))
	if !static {
		return TritMaybe
	}
	if attrOpMatches(s.op, val, s.val, s.ci) {
		return TritYes
	}
	return TritNo
}

// attrOpMatches applies a Selectors L4 attribute operator to static strings.
func attrOpMatches(op attrOp, val, want string, ci bool) bool {
	if ci {
		val = strings.ToLower(val)
		want = strings.ToLower(want)
	}
	switch op {
	case attrEquals:
		return val == want
	case attrIncludes:
		if want == "" || strings.ContainsFunc(want, isHTMLSpace) {
			return false
		}
		for _, tok := range strings.FieldsFunc(val, isHTMLSpace) {
			if tok == want {
				return true
			}
		}
		return false
	case attrDashMatch:
		return val == want || strings.HasPrefix(val, want+"-")
	case attrPrefix:
		return want != "" && strings.HasPrefix(val, want)
	case attrSuffix:
		return want != "" && strings.HasSuffix(val, want)
	case attrSubstring:
		return want != "" && strings.Contains(val, want)
	default:
		return false
	}
}

// findAttr locates a statically-named attribute on el. It returns the
// attribute node and a Trit presence: Yes when a static-key attribute of that
// name is present, Maybe when the element carries attributes whose names
// cannot be determined statically (dynamic keys, or template control flow in
// attribute position), and No when the attribute is definitely absent.
func findAttr(el *template.HTMLElementNode, name string) (*template.HTMLAttributeNode, Trit) {
	uncertain := false
	for _, a := range el.Attr {
		attr, ok := a.(*template.HTMLAttributeNode)
		if !ok {
			// ActionNode / IfNode / RangeNode in attribute position: the set of
			// attributes is not statically known.
			uncertain = true
			continue
		}
		if attr.Namespace != "" {
			continue
		}
		key, static := staticText(attr.Key)
		if !static {
			uncertain = true
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return attr, TritYes
		}
	}
	if uncertain {
		return nil, TritMaybe
	}
	return nil, TritNo
}

// staticText concatenates the text of nodes when every node is a static
// TextNode. It returns ok=false if any node is dynamic (e.g. an ActionNode).
func staticText(nodes []template.Node) (string, bool) {
	var sb strings.Builder
	for _, n := range nodes {
		t, ok := n.(*template.TextNode)
		if !ok {
			return "", false
		}
		sb.Write(t.Text)
	}
	return sb.String(), true
}

// valuePart is one segment of an attribute value: either static text or a
// dynamic template action. The htt parser keeps quote delimiters in the value
// text; attrValueParts strips them.
type valuePart struct {
	text    string
	dynamic bool
}

// attrValueParts returns the attribute value as a sequence of static and
// dynamic parts, with the surrounding quote delimiters removed.
func attrValueParts(attr *template.HTMLAttributeNode) []valuePart {
	parts := make([]valuePart, 0, len(attr.Value))
	for _, v := range attr.Value {
		if t, ok := v.(*template.TextNode); ok {
			parts = append(parts, valuePart{text: string(t.Text)})
		} else {
			parts = append(parts, valuePart{dynamic: true})
		}
	}
	if len(parts) > 0 && !parts[0].dynamic {
		parts[0].text = trimLeadingQuote(parts[0].text)
	}
	if n := len(parts); n > 0 && !parts[n-1].dynamic {
		parts[n-1].text = trimTrailingQuote(parts[n-1].text)
	}
	return parts
}

func trimLeadingQuote(s string) string {
	if len(s) > 0 && (s[0] == '"' || s[0] == '\'') {
		return s[1:]
	}
	return s
}

func trimTrailingQuote(s string) string {
	if len(s) > 0 && (s[len(s)-1] == '"' || s[len(s)-1] == '\'') {
		return s[:len(s)-1]
	}
	return s
}

// staticValue concatenates the text of value parts when every part is static.
// It returns ok=false when any part is dynamic.
func staticValue(parts []valuePart) (string, bool) {
	var sb strings.Builder
	for _, p := range parts {
		if p.dynamic {
			return "", false
		}
		sb.WriteString(p.text)
	}
	return sb.String(), true
}

// classToken is one whitespace-separated token of a class attribute. dynamic
// is true when the token contains, or is glued to, a template action, so its
// runtime text is not statically known.
type classToken struct {
	text    string
	dynamic bool
}

// classTokens splits an attribute value into whitespace-separated tokens,
// marking any token that touches a dynamic part as dynamic.
func classTokens(value []valuePart) []classToken {
	var tokens []classToken
	var cur strings.Builder
	curDynamic := false
	curActive := false
	flush := func() {
		if curActive {
			tokens = append(tokens, classToken{text: cur.String(), dynamic: curDynamic})
			cur.Reset()
			curDynamic = false
			curActive = false
		}
	}
	for _, p := range value {
		if p.dynamic {
			// A dynamic part glues to the current token (or starts one), marking it
			// dynamic. The next text run continues the same token.
			curDynamic = true
			curActive = true
			continue
		}
		for _, r := range p.text {
			if isHTMLSpace(r) {
				flush()
				continue
			}
			cur.WriteRune(r)
			curActive = true
		}
	}
	flush()
	return tokens
}

func isHTMLSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\f', '\r':
		return true
	default:
		return false
	}
}
