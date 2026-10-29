package analysis

import (
	"errors"
	"fmt"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// CheckStyleAttributes reports style attribute declarations that browsers
// drop: unknown properties and values that do not match the property's
// syntax. Declarations built with template actions are not checked.
func CheckStyleAttributes(t *template.Tree) Diagnostics {
	if t == nil || t.Root == nil {
		return nil
	}
	var diags Diagnostics
	template.Walk(t, func(_ *template.Tree, n template.Node) {
		attr, ok := n.(*template.HTMLAttributeNode)
		if !ok || len(attr.Value) == 0 || !strings.EqualFold(staticAttrName(attr), "style") {
			return
		}
		css := templateCSS(attr.Value)
		css.unquote()
		diags = append(diags, checkDeclarations(css)...)
	})
	diags.SortByPos()
	return diags
}

// StyleDeclaration is a declaration in a style attribute, with reference
// documentation for its property.
type StyleDeclaration struct {
	Property string
	Value    string
	Pos, End lex.Pos // the declaration, through its terminating semicolon

	// Known reports whether the property is in the CSS reference data; the
	// fields below are set only when it is.
	Known     bool
	Syntax    string
	Initial   string // for a shorthand, the longhands it takes initial values from
	Shorthand bool
	Inherited bool
	Status    string // "standard", "nonstandard", "experimental" or "obsolete"
	URL       string // MDN reference page, or ""

	// Problems are the diagnostics CheckStyleAttributes reports for the
	// declaration.
	Problems Diagnostics
}

// StyleDeclarationAt returns the declaration at pos in a style attribute or
// in a style rule of a <style> element.
func StyleDeclarationAt(t *template.Tree, pos lex.Pos) (StyleDeclaration, bool) {
	if t == nil || t.Root == nil {
		return StyleDeclaration{}, false
	}
	for _, block := range append(styleAttributes([]*template.Tree{t}), styleBlocks([]*template.Tree{t}, nil)...) {
		css := block.css
		if pos < css.base || pos >= css.pos(len(css.text)) {
			continue
		}
		if block.attribute {
			ast, err := csstree.Parse(css.text, csstree.ParseOptions{Context: "declarationList", Positions: true})
			if err != nil {
				continue
			}
			if list, ok := ast.(*csstree.DeclarationList); ok {
				return declarationAt(css, list.Children, len(css.text), pos, checkDeclarations(css))
			}
			continue
		}
		sheet, ok := parseStylesheet(css)
		if !ok {
			continue
		}
		var (
			found    StyleDeclaration
			isFound  bool
			problems = checkStyleBlock(css)
		)
		eachStyleRuleBlock(sheet.Children, func(block *csstree.Block) bool {
			found, isFound = declarationAt(css, block.Children, blockEnd(css, block), pos, problems)
			return isFound
		})
		return found, isFound
	}
	return StyleDeclaration{}, false
}

// eachStyleRuleBlock calls fn with the block of each style rule in list,
// including rules in grouping at-rules and nested rules, until fn returns
// true. It reports whether fn returned true.
func eachStyleRuleBlock(list *csstree.List[csstree.Node], fn func(*csstree.Block) bool) bool {
	for item := list.Head(); item != nil; item = item.Next {
		var block *csstree.Block
		switch n := item.Data.(type) {
		case *csstree.Atrule:
			if n.Block != nil && groupingAtRules[strings.ToLower(n.Name)] && eachStyleRuleBlock(n.Block.Children, fn) {
				return true
			}
			continue
		case *csstree.Rule:
			block = n.Block
		}
		if block != nil && (fn(block) || eachStyleRuleBlock(block.Children, fn)) {
			return true
		}
	}
	return false
}

// declarationAt returns the declaration in list, which ends at listEnd,
// whose range holds pos. problems are the diagnostics of the CSS holding
// list.
func declarationAt(css cssText, list *csstree.List[csstree.Node], listEnd int, pos lex.Pos, problems Diagnostics) (StyleDeclaration, bool) {
	for item := list.Head(); item != nil; item = item.Next {
		decl, ok := item.Data.(*csstree.Declaration)
		if !ok || decl.Loc == nil {
			continue
		}
		start, end := decl.Loc.Start.Offset, declarationEnd(css.text, decl, item.Next, listEnd)
		if pos < css.pos(start) || pos >= css.pos(end) {
			continue
		}
		d := StyleDeclaration{
			Property: decl.Property,
			Pos:      css.pos(start),
			End:      css.pos(end),
		}
		if colon := strings.IndexByte(css.text[start:decl.Loc.End.Offset], ':'); colon >= 0 {
			d.Value = strings.TrimSpace(css.text[start+colon+1 : decl.Loc.End.Offset])
		}
		if property, known := csstree.LookupMDNProperty(decl.Property); known {
			d.Known = true
			d.Syntax, d.Initial, d.Shorthand = property.Syntax, property.Initial, property.Shorthand
			d.Inherited, d.Status, d.URL = property.Inherited, property.Status, property.URL
		}
		for _, problem := range problems {
			if problem.Pos >= d.Pos && problem.Pos < d.End {
				d.Problems = append(d.Problems, problem)
			}
		}
		return d, true
	}
	return StyleDeclaration{}, false
}

// staticAttrName returns the attribute name when every part of it is static
// text, and "" otherwise.
func staticAttrName(attr *template.HTMLAttributeNode) string {
	var sb strings.Builder
	for _, k := range attr.Key {
		text, ok := k.(*template.TextNode)
		if !ok {
			return ""
		}
		sb.Write(text.Text)
	}
	return sb.String()
}

// checkDeclarations matches each static declaration of a declaration list
// against its property's syntax.
func checkDeclarations(css cssText) Diagnostics {
	ast, err := csstree.Parse(css.text, csstree.ParseOptions{Context: "declarationList", Positions: true})
	if err != nil {
		return nil
	}
	list, ok := ast.(*csstree.DeclarationList)
	if !ok {
		return nil
	}
	return checkDeclarationList(css, list.Children, len(css.text))
}

// checkDeclarationList matches the static declarations in list against
// their properties' syntaxes; list is a declaration list or the contents of
// a style rule's block, and end is the offset where the list ends.
func checkDeclarationList(css cssText, list *csstree.List[csstree.Node], end int) Diagnostics {
	lexer := csstree.DefaultLexer()
	var diags Diagnostics
	for item := list.Head(); item != nil; item = item.Next {
		decl, ok := item.Data.(*csstree.Declaration)
		if !ok || decl.Loc == nil {
			continue
		}
		if css.dynamic(decl.Loc.Start.Offset, declarationEnd(css.text, decl, item.Next, end)) {
			continue
		}
		result := lexer.MatchDeclaration(decl)
		var (
			unknown  *csstree.SyntaxReferenceError
			mismatch *csstree.SyntaxMatchError
		)
		switch {
		case errors.As(result.Error, &unknown):
			if strings.HasPrefix(decl.Property, "-") {
				continue // vendor-prefixed properties are browser specific
			}
			diags = append(diags, Diagnostic{
				Pos:      css.pos(decl.Loc.Start.Offset),
				End:      css.pos(decl.Loc.End.Offset),
				Severity: SeverityWarning,
				Code:     template.DiagUnknownCSSProperty,
				Message:  fmt.Sprintf(msgUnknownCSSProperty, decl.Property),
			})
		case errors.As(result.Error, &mismatch):
			start, end := decl.Loc.Start.Offset, decl.Loc.End.Offset
			if mismatch.Loc.End.Offset > mismatch.Loc.Start.Offset {
				start, end = mismatch.Loc.Start.Offset, mismatch.Loc.End.Offset
			}
			diags = append(diags, Diagnostic{
				Pos:      css.pos(start),
				End:      css.pos(end),
				Severity: SeverityWarning,
				Code:     template.DiagInvalidCSSValue,
				Message:  fmt.Sprintf(msgInvalidCSSValue, mismatch.CSS, decl.Property, mismatch.Syntax),
			})
		}
	}
	return diags
}

// declarationEnd returns the offset of the semicolon ending decl, or of the
// next item (or the list's end) when there is no semicolon before it. A
// declaration's location ends at its last value token, so a template action
// blanked to a comment at the end of the value lies between the location's
// end and this offset.
func declarationEnd(text string, decl *csstree.Declaration, next *csstree.ListItem[csstree.Node], listEnd int) int {
	limit := listEnd
	if next != nil {
		if loc := next.Data.Location(); loc != nil {
			limit = loc.Start.Offset
		}
	}
	end := decl.Loc.End.Offset
	if i := strings.IndexByte(text[end:limit], ';'); i >= 0 {
		return end + i
	}
	return limit
}

// cssText is CSS cut from a template. Static text keeps its template byte
// offset relative to base; template actions and control flow are blanked to
// CSS comments of the same length and recorded as gaps.
type cssText struct {
	base lex.Pos
	text string
	gaps []span
}

// span is a half-open byte range [start, end) of cssText.text.
type span struct{ start, end int }

// templateCSS builds the CSS text spanning nodes, which are consecutive
// parts of an attribute value or the children of a raw-text element.
func templateCSS(nodes []template.Node) cssText {
	base, end := nodes[0].Start(), nodes[len(nodes)-1].End()
	buf := []byte(strings.Repeat(" ", int(end-base)))
	static := make([]bool, len(buf))
	for _, n := range nodes {
		eachText(n, func(text *template.TextNode) {
			offset := int(text.Start() - base)
			copy(buf[offset:], text.String())
			for i := offset; i < int(text.End()-base); i++ {
				static[i] = true
			}
		})
	}
	css := cssText{base: base}
	for i := 0; i < len(buf); {
		if static[i] {
			i++
			continue
		}
		j := i
		for j < len(buf) && !static[j] {
			j++
		}
		if j-i >= len("/**/") {
			buf[i], buf[i+1], buf[j-2], buf[j-1] = '/', '*', '*', '/'
		}
		css.gaps = append(css.gaps, span{i, j})
		i = j
	}
	css.text = string(buf)
	return css
}

// eachText calls fn for each text node in n, including text in the branches
// of if, range, and with actions.
func eachText(n template.Node, fn func(*template.TextNode)) {
	switch n := n.(type) {
	case *template.TextNode:
		fn(n)
	case *template.ListNode:
		for _, c := range n.Nodes {
			eachText(c, fn)
		}
	case *template.IfNode:
		eachBranchText(n.List, n.ElseList, fn)
	case *template.RangeNode:
		eachBranchText(n.List, n.ElseList, fn)
	case *template.WithNode:
		eachBranchText(n.List, n.ElseList, fn)
	}
}

func eachBranchText(list, elseList *template.ListNode, fn func(*template.TextNode)) {
	if list != nil {
		eachText(list, fn)
	}
	if elseList != nil {
		eachText(elseList, fn)
	}
}

// unquote blanks the quotes around an attribute value; the source of an
// attribute value's text nodes includes them.
func (c *cssText) unquote() {
	n := len(c.text)
	if n < 2 || (c.text[0] != '"' && c.text[0] != '\'') || c.text[n-1] != c.text[0] {
		return
	}
	c.text = " " + c.text[1:n-1] + " "
}

// dynamic reports whether [start, end) overlaps a template action.
func (c cssText) dynamic(start, end int) bool {
	for _, g := range c.gaps {
		if g.start < end && start < g.end {
			return true
		}
	}
	return false
}

// source returns the trimmed CSS text at loc.
func (c cssText) source(loc *csstree.Location) string {
	return strings.TrimSpace(c.text[loc.Start.Offset:loc.End.Offset])
}

// pos maps an offset in c.text to a template position.
func (c cssText) pos(offset int) lex.Pos { return c.base + lex.Pos(offset) }
