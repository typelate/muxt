package selectors

import "github.com/typelate/muxt/internal/template"

// combinator joins two selectors by a structural relationship.
type combinator byte

const (
	descendant combinator = ' ' // "A B"
	child      combinator = '>' // "A > B"
	adjacent   combinator = '+' // "A + B"
	sibling    combinator = '~' // "A ~ B"
)

// combinatorSelector matches when right matches the subject node and left
// matches a node in the relationship named by comb.
type combinatorSelector struct {
	left  matcher
	comb  combinator
	right matcher
}

func (c combinatorSelector) match(n template.Node) Trit {
	r := c.right.match(n)
	if r == TritNo {
		return TritNo
	}
	var rel Trit
	switch c.comb {
	case child:
		if p := parentElement(n); p != nil {
			rel = c.left.match(p)
		}
	case descendant:
		for _, a := range ancestorElements(n) {
			rel = rel.Or(c.left.match(a))
			if rel == TritYes {
				break
			}
		}
	case sibling:
		rel = c.generalSibling(n)
	case adjacent:
		rel = c.adjacentSibling(n)
	}
	return r.And(rel)
}

func (c combinatorSelector) generalSibling(n template.Node) Trit {
	result := TritNo
	for _, s := range precedingSiblings(n) {
		m := c.left.match(s.el)
		if s.conditional {
			m = m.And(TritMaybe)
		}
		result = result.Or(m)
		if result == TritYes {
			return TritYes
		}
	}
	return result
}

func (c combinatorSelector) adjacentSibling(n template.Node) Trit {
	sibs := precedingSiblings(n)
	result := TritNo
	skippedConditional := false
	for i := len(sibs) - 1; i >= 0; i-- {
		s := sibs[i]
		m := c.left.match(s.el)
		if s.conditional {
			// A conditional sibling may or may not be the adjacent predecessor.
			result = result.Or(m.And(TritMaybe))
			skippedConditional = true
			continue
		}
		// A certainly-present sibling is the adjacent predecessor — unless a
		// conditional sibling between it and n is present at runtime.
		adjacency := TritYes
		if skippedConditional {
			adjacency = TritMaybe
		}
		result = result.Or(m.And(adjacency))
		break
	}
	return result
}

// parentOf returns a node's lexical parent via the optional Parent method that
// structural htt nodes implement.
func parentOf(n template.Node) template.Node {
	if p, ok := n.(interface{ Parent() template.Node }); ok {
		return p.Parent()
	}
	return nil
}

// parentElement returns the nearest ancestor element, treating template control
// flow (lists, if/range/with) as transparent. It returns nil when the nearest
// container is the document fragment.
func parentElement(n template.Node) *template.HTMLElementNode {
	for p := parentOf(n); p != nil; p = parentOf(p) {
		switch x := p.(type) {
		case *template.HTMLElementNode:
			return x
		case *template.HTMLDocumentFragmentNode:
			return nil
		}
	}
	return nil
}

// ancestorElements returns all ancestor elements from nearest to furthest.
func ancestorElements(n template.Node) []*template.HTMLElementNode {
	var out []*template.HTMLElementNode
	for p := parentOf(n); p != nil; p = parentOf(p) {
		if el, ok := p.(*template.HTMLElementNode); ok {
			out = append(out, el)
		}
	}
	return out
}

// siblingElement is an element child of a container, flattened through control
// flow. conditional is true when the element appears inside an if/range/with
// body, so it may be absent (or repeated) at runtime.
type siblingElement struct {
	el          *template.HTMLElementNode
	conditional bool
}

// elementContainer returns the nearest ancestor that holds element children:
// an element or the document fragment.
func elementContainer(n template.Node) template.Node {
	for p := parentOf(n); p != nil; p = parentOf(p) {
		switch p.(type) {
		case *template.HTMLElementNode, *template.HTMLDocumentFragmentNode:
			return p
		}
	}
	return nil
}

// precedingSiblings returns the element siblings that appear before n in its
// container, flattening template control flow.
func precedingSiblings(n template.Node) []siblingElement {
	container := elementContainer(n)
	if container == nil {
		return nil
	}
	sibs := flattenedSiblings(container)
	for i, s := range sibs {
		if template.Node(s.el) == n {
			return sibs[:i]
		}
	}
	return nil
}

// flattenedSiblings collects the element children of container in source order,
// descending through lists and if/range/with bodies (marking those elements
// conditional) but not through nested elements.
func flattenedSiblings(container template.Node) []siblingElement {
	var out []siblingElement
	var walk func(children []template.Node, conditional bool)
	walk = func(children []template.Node, conditional bool) {
		for _, c := range children {
			switch x := c.(type) {
			case *template.HTMLElementNode:
				out = append(out, siblingElement{el: x, conditional: conditional})
			case *template.ListNode:
				walk(x.Nodes, conditional)
			case *template.IfNode:
				walkBranch(walk, x.List, x.ElseList)
			case *template.RangeNode:
				walkBranch(walk, x.List, x.ElseList)
			case *template.WithNode:
				walkBranch(walk, x.List, x.ElseList)
			}
		}
	}
	switch c := container.(type) {
	case *template.HTMLElementNode:
		walk(c.Children, false)
	case *template.HTMLDocumentFragmentNode:
		walk(c.Children, false)
	}
	return out
}

func walkBranch(walk func([]template.Node, bool), list, elseList *template.ListNode) {
	if list != nil {
		walk(list.Nodes, true)
	}
	if elseList != nil {
		walk(elseList.Nodes, true)
	}
}
