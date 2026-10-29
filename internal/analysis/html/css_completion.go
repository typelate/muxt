package analysis

import (
	"maps"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// CSSCompletion is what may be typed at a position in a style attribute or
// a <style> element.
type CSSCompletion struct {
	// Property is the property whose value is being typed, or "" when a
	// property name is being typed.
	Property string
	// Partial is the identifier typed so far, which starts at Start.
	Partial string
	Start   lex.Pos
	// Candidates are the property names or value keywords, sorted.
	Candidates []string
}

// CSSCompletionAt returns the CSS completion at pos in t: property names
// where a declaration starts, and the keywords of the property's syntax in
// its value. Selectors and template actions get no completion.
func CSSCompletionAt(t *template.Tree, pos lex.Pos) (CSSCompletion, bool) {
	if t == nil || t.Root == nil {
		return CSSCompletion{}, false
	}
	trees := []*template.Tree{t}
	for _, block := range append(styleAttributes(trees), styleBlocks(trees, nil)...) {
		css := block.css
		if pos < css.base || pos > css.pos(len(css.text)) {
			continue
		}
		return completionIn(css, int(pos-css.base), block.attribute)
	}
	return CSSCompletion{}, false
}

func completionIn(css cssText, offset int, declarationList bool) (CSSCompletion, bool) {
	before := css.text[:offset]
	start := offset
	for start > 0 && isIdentByte(before[start-1]) {
		start--
	}
	if css.dynamic(start-1, offset+1) {
		return CSSCompletion{}, false
	}
	if !declarationList && strings.Count(before, "{") <= strings.Count(before, "}") {
		return CSSCompletion{}, false // a selector or at-rule prelude
	}
	segment := before[strings.LastIndexAny(before[:start], ";{}")+1 : start]
	c := CSSCompletion{Partial: before[start:], Start: css.pos(start)}
	if property, _, isValue := strings.Cut(segment, ":"); isValue {
		c.Property = strings.TrimSpace(property)
		if c.Property == "" || strings.IndexFunc(c.Property, func(r rune) bool { return r > 0x7f || !isIdentByte(byte(r)) }) >= 0 {
			return CSSCompletion{}, false // a pseudo-class of a nested selector, not a declaration
		}
		c.Candidates = valueKeywords(c.Property)
		return c, true
	}
	if strings.TrimSpace(segment) != "" {
		return CSSCompletion{}, false // a nested selector
	}
	c.Candidates = csstree.MDNPropertyNames()
	return c, true
}

func isIdentByte(b byte) bool {
	return b == '-' || b == '_' || ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') || ('0' <= b && b <= '9')
}

// valueKeywords returns the keywords a value of property may contain, from
// its syntax and the syntaxes it refers to, with the CSS-wide keywords.
func valueKeywords(property string) []string {
	lexer := csstree.DefaultLexer()
	keywords := make(map[string]bool)
	for _, k := range lexer.CSSWideKeywords() {
		keywords[k] = true
	}
	visited := make(map[string]bool)
	var collect func(d *csstree.Descriptor)
	collect = func(d *csstree.Descriptor) {
		if d == nil {
			return
		}
		syntax, err := d.Syntax()
		if err != nil || syntax == nil {
			return
		}
		var references []*csstree.Descriptor
		walkErr := definitionsyntax.Walk(syntax, definitionsyntax.WalkOptions{Enter: func(n definitionsyntax.Node) {
			switch n := n.(type) {
			case *definitionsyntax.Keyword:
				keywords[strings.ToLower(n.Name)] = true
			case *definitionsyntax.Type:
				if key := "<" + n.Name + ">"; !visited[key] {
					visited[key] = true
					references = append(references, lexer.GetType(n.Name))
				}
			case *definitionsyntax.Property:
				if key := "<'" + n.Name + "'>"; !visited[key] {
					visited[key] = true
					references = append(references, lexer.GetProperty(n.Name, false))
				}
			}
		}})
		if walkErr != nil {
			return
		}
		for _, r := range references {
			collect(r)
		}
	}
	collect(lexer.GetProperty(property, true))
	return slices.Sorted(maps.Keys(keywords))
}
