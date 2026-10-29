package selectors

import (
	"iter"

	"github.com/typelate/muxt/internal/template"
)

// QuerySelectorAll returns an iterator over the descendant elements of root
// that the selector matches, in document order. The yielded Trit is the match
// certainty combined with the element's runtime presence: an element inside an
// {{if}}/{{range}}/{{with}} body yields TritMaybe even when the selector
// matches its static attributes. Elements that cannot match (TritNo) are not
// yielded, so the second value is always TritYes or TritMaybe. The root node
// itself is not tested.
//
// Traversal does not enter DefineNode or BlockNode bodies: those are detached
// templates, reached by following {{template}}/{{block}} calls rather than by
// lexical descent.
func (s Selector) QuerySelectorAll(root template.Node) iter.Seq2[*template.HTMLElementNode, Trit] {
	return func(yield func(*template.HTMLElementNode, Trit) bool) {
		walkChildren(root, false, func(el *template.HTMLElementNode, conditional bool) bool {
			t := s.matchPresence(el, conditional)
			if t == TritNo {
				return false
			}
			// walkChildren stops when fn returns true; stop when yield asks to.
			return !yield(el, t)
		})
	}
}

// QuerySelector returns the first descendant element of root that the selector
// matches, in document order, with its Trit. When nothing matches it returns
// (nil, TritNo).
func (s Selector) QuerySelector(root template.Node) (*template.HTMLElementNode, Trit) {
	for el, t := range s.QuerySelectorAll(root) {
		return el, t
	}
	return nil, TritNo
}

// Closest returns the nearest element at or above start that the selector
// matches, walking up the lexical ancestors, with its Trit. An ancestor is
// present whenever start is, so the Trit is the plain match certainty. When
// nothing matches it returns (nil, TritNo).
func (s Selector) Closest(start template.Node) (*template.HTMLElementNode, Trit) {
	for n := start; n != nil; n = parentOf(n) {
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			continue
		}
		if t := s.trit(el); t != TritNo {
			return el, t
		}
	}
	return nil, TritNo
}

// matchPresence combines the selector match for el with el's runtime presence.
func (s Selector) matchPresence(el *template.HTMLElementNode, conditional bool) Trit {
	t := s.trit(el)
	if conditional {
		t = t.And(TritMaybe)
	}
	return t
}

// walkEl visits n in document order: if n is an element it is reported, then
// its children are walked; otherwise its structural children are walked.
// conditional is true once descent has passed through an if/range/with body.
// fn returns true to stop the walk.
func walkEl(n template.Node, conditional bool, fn func(*template.HTMLElementNode, bool) bool) bool {
	if el, ok := n.(*template.HTMLElementNode); ok {
		if fn(el, conditional) {
			return true
		}
		return walkChildren(el, conditional, fn)
	}
	return walkChildren(n, conditional, fn)
}

// walkChildren walks the structural children of n. Children of an if/range/with
// body are marked conditional. DefineNode and BlockNode bodies are not entered.
func walkChildren(n template.Node, conditional bool, fn func(*template.HTMLElementNode, bool) bool) bool {
	switch x := n.(type) {
	case *template.HTMLElementNode:
		return walkEach(x.Children, conditional, fn)
	case *template.ListNode:
		return walkEach(x.Nodes, conditional, fn)
	case *template.HTMLDocumentFragmentNode:
		return walkEach(x.Children, conditional, fn)
	case *template.IfNode:
		return walkBranchEach(x.List, x.ElseList, fn)
	case *template.RangeNode:
		return walkBranchEach(x.List, x.ElseList, fn)
	case *template.WithNode:
		return walkBranchEach(x.List, x.ElseList, fn)
	default:
		return false
	}
}

func walkEach(children []template.Node, conditional bool, fn func(*template.HTMLElementNode, bool) bool) bool {
	for _, c := range children {
		if walkEl(c, conditional, fn) {
			return true
		}
	}
	return false
}

func walkBranchEach(list, elseList *template.ListNode, fn func(*template.HTMLElementNode, bool) bool) bool {
	if list != nil && walkEach(list.Nodes, true, fn) {
		return true
	}
	if elseList != nil && walkEach(elseList.Nodes, true, fn) {
		return true
	}
	return false
}
