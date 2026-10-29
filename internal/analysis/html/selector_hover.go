package analysis

import (
	"fmt"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
	"github.com/typelate/muxt/internal/template/selectors"
)

// Specificity is a selector's specificity: its ID, class, and type
// counts.
type Specificity [3]int

func (s Specificity) String() string { return fmt.Sprintf("(%d, %d, %d)", s[0], s[1], s[2]) }

func (s Specificity) add(o Specificity) Specificity {
	return Specificity{s[0] + o[0], s[1] + o[1], s[2] + o[2]}
}

func (s Specificity) less(o Specificity) bool {
	for i := range s {
		if s[i] != o[i] {
			return s[i] < o[i]
		}
	}
	return false
}

// SelectorInfo describes a selector in a <style> element.
type SelectorInfo struct {
	Text        string
	Pos, End    lex.Pos
	Specificity Specificity
	// Matches counts the elements of the template the selector matches;
	// ConditionalMatches counts those it may match, in some branches or
	// depending on dynamic attributes.
	Matches, ConditionalMatches int
	// Unchecked reports that htt cannot match the selector, so the counts
	// are zero.
	Unchecked bool
}

// SelectorAt returns the selector at pos in a <style> element of t, with
// its specificity and the elements of t it matches.
func SelectorAt(t *template.Tree, pos lex.Pos) (SelectorInfo, bool) {
	if t == nil || t.Root == nil {
		return SelectorInfo{}, false
	}
	for _, block := range styleBlocks([]*template.Tree{t}, nil) {
		css := block.css
		if pos < css.base || pos >= css.pos(len(css.text)) {
			continue
		}
		sheet, ok := parseStylesheet(css)
		if !ok {
			continue
		}
		var (
			info  SelectorInfo
			found bool
		)
		eachSelector(css, sheet.Children, nestingContext{}, func(sel *csstree.Selector, parent nestingContext) bool {
			if pos < css.pos(sel.Loc.Start.Offset) || pos >= css.pos(sel.Loc.End.Offset) {
				return false
			}
			info, found = describeSelector(css, sel, parent, templateRoots([]*template.Tree{t})), true
			return true
		})
		if found {
			return info, true
		}
	}
	return SelectorInfo{}, false
}

func describeSelector(css cssText, sel *csstree.Selector, parent nestingContext, roots []template.Node) SelectorInfo {
	info := SelectorInfo{
		Text:        css.source(sel.Loc),
		Pos:         css.pos(sel.Loc.Start.Offset),
		End:         css.pos(sel.Loc.End.Offset),
		Specificity: selectorSpecificity(sel, parent.specificity),
	}
	query, _, ok := resolveSelector(sel, parent.query)
	if !ok {
		info.Unchecked = true
		return info
	}
	compiled, err := selectors.Compile(query)
	if err != nil {
		info.Unchecked = true
		return info
	}
	for _, root := range roots {
		for _, certainty := range compiled.QuerySelectorAll(root) {
			if certainty == selectors.TritYes {
				info.Matches++
			} else {
				info.ConditionalMatches++
			}
		}
	}
	return info
}

// nestingContext is what the selectors of a nested style rule are relative
// to: the enclosing rule's selector list, resolved for the selectors
// package, and its specificity, which a nesting selector (&) takes.
type nestingContext struct {
	query       string
	specificity Specificity
}

// eachSelector calls fn with each static selector of the style rules in
// list, including rules in grouping at-rules and nested rules. It stops when
// fn returns true, and reports whether it stopped.
func eachSelector(css cssText, list *csstree.List[csstree.Node], parent nestingContext, fn func(*csstree.Selector, nestingContext) bool) bool {
	for item := list.Head(); item != nil; item = item.Next {
		switch n := item.Data.(type) {
		case *csstree.Atrule:
			if n.Block != nil && groupingAtRules[strings.ToLower(n.Name)] && eachSelector(css, n.Block.Children, parent, fn) {
				return true
			}
		case *csstree.Rule:
			if eachRuleSelector(css, n, parent, fn) {
				return true
			}
		}
	}
	return false
}

func eachRuleSelector(css cssText, rule *csstree.Rule, parent nestingContext, fn func(*csstree.Selector, nestingContext) bool) bool {
	list, ok := rule.Prelude.(*csstree.SelectorList)
	if !ok || list.Loc == nil || css.dynamic(list.Loc.Start.Offset, preludeEnd(rule)) {
		return false
	}
	var (
		nested   nestingContext
		resolved []string
	)
	for item := list.Children.Head(); item != nil; item = item.Next {
		sel, ok := item.Data.(*csstree.Selector)
		if !ok || sel.Loc == nil {
			return false
		}
		if fn(sel, parent) {
			return true
		}
		query, _, ok := resolveSelector(sel, parent.query)
		if !ok {
			return false
		}
		resolved = append(resolved, query)
		if s := selectorSpecificity(sel, parent.specificity); nested.specificity.less(s) {
			nested.specificity = s
		}
	}
	nested.query = strings.Join(resolved, ", ")
	return rule.Block != nil && eachSelector(css, rule.Block.Children, nested, fn)
}

// selectorSpecificity computes the specificity of sel as Selectors Level 4
// defines it. nesting is the specificity a nesting selector (&) takes.
func selectorSpecificity(sel *csstree.Selector, nesting Specificity) Specificity {
	return compoundSpecificity(sel.Children, nesting)
}

func compoundSpecificity(children *csstree.List[csstree.Node], nesting Specificity) Specificity {
	var s Specificity
	if children == nil {
		return s
	}
	for item := children.Head(); item != nil; item = item.Next {
		switch n := item.Data.(type) {
		case *csstree.IDSelector:
			s[0]++
		case *csstree.ClassSelector, *csstree.AttributeSelector:
			s[1]++
		case *csstree.TypeSelector:
			if n.Name != "*" && !strings.HasSuffix(n.Name, "|*") {
				s[2]++
			}
		case *csstree.PseudoElementSelector:
			s[2]++
		case *csstree.NestingSelector:
			s = s.add(nesting)
		case *csstree.PseudoClassSelector:
			s = s.add(pseudoClassSpecificity(n, nesting))
		}
	}
	return s
}

func pseudoClassSpecificity(n *csstree.PseudoClassSelector, nesting Specificity) Specificity {
	switch strings.ToLower(n.Name) {
	case "where":
		return Specificity{}
	case "before", "after", "first-line", "first-letter": // legacy pseudo-elements
		return Specificity{0, 0, 1}
	case "is", "not", "has", "matches", "-moz-any", "-webkit-any":
		return argumentSpecificity(n.Children, nesting)
	case "nth-child", "nth-last-child":
		return Specificity{0, 1, 0}.add(argumentSpecificity(n.Children, nesting))
	}
	return Specificity{0, 1, 0}
}

// argumentSpecificity returns the greatest specificity of the selectors in
// a pseudo-class's arguments.
func argumentSpecificity(children *csstree.List[csstree.Node], nesting Specificity) Specificity {
	var greatest Specificity
	consider := func(list *csstree.SelectorList) {
		if list == nil {
			return
		}
		for item := list.Children.Head(); item != nil; item = item.Next {
			if sel, ok := item.Data.(*csstree.Selector); ok {
				if s := selectorSpecificity(sel, nesting); greatest.less(s) {
					greatest = s
				}
			}
		}
	}
	if children == nil {
		return greatest
	}
	for item := children.Head(); item != nil; item = item.Next {
		switch n := item.Data.(type) {
		case *csstree.SelectorList:
			consider(n)
		case *csstree.Nth:
			consider(n.Selector)
		}
	}
	return greatest
}
