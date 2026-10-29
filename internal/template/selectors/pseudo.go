package selectors

import (
	"strings"

	"github.com/typelate/muxt/internal/template"
)

// maybePseudo is an interactive/runtime pseudo-class (:hover, :checked, ...)
// that has no static meaning on template source: any element may match at
// runtime, so it yields TritMaybe.
type maybePseudo struct{}

func (maybePseudo) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); !ok {
		return TritNo
	}
	return TritMaybe
}

// interactivePseudos are the runtime pseudo-classes recognized (and reduced to
// TritMaybe). Functional forms (:lang, :dir) are listed without parentheses.
var interactivePseudos = map[string]bool{
	"hover": true, "active": true, "focus": true, "focus-visible": true,
	"focus-within": true, "checked": true, "enabled": true, "disabled": true,
	"required": true, "optional": true, "valid": true, "invalid": true,
	"in-range": true, "out-of-range": true, "link": true, "visited": true,
	"target": true, "lang": true, "dir": true,
}

// pseudoNot is the :not(...) negation pseudo-class.
type pseudoNot struct {
	inner matcher
}

func (p pseudoNot) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); !ok {
		return TritNo
	}
	return p.inner.match(n).Not()
}

// isPseudo is :is(...) / :where(...): matches when the element matches any
// selector in the list. :where differs only in specificity, which a static
// analyzer does not track, so the two are identical here.
type isPseudo struct {
	inner matcher
}

func (s isPseudo) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); !ok {
		return TritNo
	}
	return s.inner.match(n)
}

// firstChild is the :first-child pseudo-class.
type firstChild struct{}

func (firstChild) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); !ok {
		return TritNo
	}
	return noSiblingCertainty(precedingSiblings(n))
}

// lastChild is the :last-child pseudo-class.
type lastChild struct{}

func (lastChild) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); !ok {
		return TritNo
	}
	return noSiblingCertainty(followingSiblings(n))
}

// noSiblingCertainty reports, given the siblings on one side, whether the node
// is the edge child: Yes when there are none, No when a certainly-present
// sibling exists, Maybe when the only siblings are conditional.
func noSiblingCertainty(sibs []siblingElement) Trit {
	if len(sibs) == 0 {
		return TritYes
	}
	for _, s := range sibs {
		if !s.conditional {
			return TritNo
		}
	}
	return TritMaybe
}

// empty is the :empty pseudo-class: an element with no element children and no
// non-whitespace text. Comments are ignored; dynamic content yields Maybe.
type empty struct{}

func (empty) match(n template.Node) Trit {
	el, ok := n.(*template.HTMLElementNode)
	if !ok {
		return TritNo
	}
	sawDynamic := false
	for _, c := range el.Children {
		switch x := c.(type) {
		case *template.HTMLElementNode:
			return TritNo
		case *template.TextNode:
			if strings.TrimSpace(string(x.Text)) != "" {
				return TritNo
			}
		case *template.HTMLCommentNode, *template.CommentNode:
			// comments do not affect emptiness
		default:
			// actions and control flow may render content at runtime
			sawDynamic = true
		}
	}
	if sawDynamic {
		return TritMaybe
	}
	return TritYes
}

// followingSiblings returns the element siblings after n in its container,
// flattening template control flow.
func followingSiblings(n template.Node) []siblingElement {
	container := elementContainer(n)
	if container == nil {
		return nil
	}
	sibs := flattenedSiblings(container)
	for i, s := range sibs {
		if template.Node(s.el) == n {
			return sibs[i+1:]
		}
	}
	return nil
}

// root is the :root pseudo-class: an element with no element parent.
type root struct{}

func (root) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); !ok {
		return TritNo
	}
	if parentElement(n) == nil {
		return TritYes
	}
	return TritNo
}

// onlyChild is the :only-child pseudo-class.
type onlyChild struct{}

func (onlyChild) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); !ok {
		return TritNo
	}
	return noSiblingCertainty(precedingSiblings(n)).And(noSiblingCertainty(followingSiblings(n)))
}

// nthChild is :nth-child(An+B); fromEnd selects :nth-last-child.
type nthChild struct {
	a, b    int
	fromEnd bool
}

func (s nthChild) match(n template.Node) Trit {
	if _, ok := n.(*template.HTMLElementNode); !ok {
		return TritNo
	}
	sibs := precedingSiblings(n)
	if s.fromEnd {
		sibs = followingSiblings(n)
	}
	if anbMatchesNone(s.a, s.b) {
		return TritNo
	}
	conditional := false
	for _, sib := range sibs {
		if sib.conditional {
			conditional = true
			break
		}
	}
	if conditional {
		if anbMatchesAll(s.a, s.b) {
			return TritYes
		}
		return TritMaybe
	}
	if anbMatches(s.a, s.b, len(sibs)+1) {
		return TritYes
	}
	return TritNo
}

// nthOfType is :nth-of-type(An+B) and its edge shorthands; it counts only
// siblings whose tag matches the subject's.
type nthOfType struct {
	a, b    int
	fromEnd bool
}

func (s nthOfType) match(n template.Node) Trit {
	el, ok := n.(*template.HTMLElementNode)
	if !ok {
		return TritNo
	}
	sibs := precedingSiblings(n)
	if s.fromEnd {
		sibs = followingSiblings(n)
	}
	if anbMatchesNone(s.a, s.b) {
		return TritNo
	}
	index := 1
	conditional := false
	for _, sib := range sibs {
		if !strings.EqualFold(sib.el.TagName, el.TagName) {
			continue
		}
		if sib.conditional {
			conditional = true
			continue
		}
		index++
	}
	if conditional {
		if anbMatchesAll(s.a, s.b) {
			return TritYes
		}
		return TritMaybe
	}
	if anbMatches(s.a, s.b, index) {
		return TritYes
	}
	return TritNo
}

// hasPseudo is :has(rel): the element has a descendant (or, with a leading
// combinator, a child/sibling) matching the relative selector.
type hasPseudo struct {
	comb combinator
	rel  matcher
}

func (s hasPseudo) match(n template.Node) Trit {
	el, ok := n.(*template.HTMLElementNode)
	if !ok {
		return TritNo
	}
	switch s.comb {
	case child:
		return s.overSiblingElements(childElements(el))
	case adjacent:
		return s.hasAdjacent(followingSiblings(n))
	case sibling:
		return s.overSiblingElements(followingSiblings(n))
	default: // descendant
		result := TritNo
		walkChildren(el, false, func(cand *template.HTMLElementNode, conditional bool) bool {
			m := s.rel.match(cand)
			if conditional {
				m = m.And(TritMaybe)
			}
			result = result.Or(m)
			return result == TritYes // stop early on a certain match
		})
		return result
	}
}

func (s hasPseudo) overSiblingElements(cands []siblingElement) Trit {
	result := TritNo
	for _, c := range cands {
		m := s.rel.match(c.el)
		if c.conditional {
			m = m.And(TritMaybe)
		}
		result = result.Or(m)
		if result == TritYes {
			return TritYes
		}
	}
	return result
}

// hasAdjacent evaluates the relative selector against the element's adjacent
// following sibling, accounting for conditional siblings that may not render:
// a conditional sibling might be the adjacent one, or might be absent so the
// next sibling becomes adjacent. It mirrors combinatorSelector.adjacentSibling
// in the forward direction.
func (s hasPseudo) hasAdjacent(sibs []siblingElement) Trit {
	result := TritNo
	skippedConditional := false
	for _, sib := range sibs {
		m := s.rel.match(sib.el)
		if sib.conditional {
			result = result.Or(m.And(TritMaybe))
			skippedConditional = true
			continue
		}
		adjacency := TritYes
		if skippedConditional {
			adjacency = TritMaybe
		}
		result = result.Or(m.And(adjacency))
		break
	}
	return result
}

// childElements returns the direct element children of el, flattening control
// flow (conditional children are marked).
func childElements(el *template.HTMLElementNode) []siblingElement {
	return flattenedSiblings(el)
}
