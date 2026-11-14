package analysis

import (
	"iter"

	"github.com/typelate/htt"
)

// Modality describes how control flow qualifies an ancestor
// relationship discovered along a WalkPath path.
type Modality int

const (
	ModalityAlways        Modality = iota // relationship holds on every render
	ModalityConditionally                 // only when an {{if}}/{{with}} branch renders
	ModalityRepeatedly                    // inside {{range}}: 0..n times per render
)

func (m Modality) String() string {
	switch m {
	case ModalityAlways:
		return "always"
	case ModalityConditionally:
		return "conditionally"
	case ModalityRepeatedly:
		return "repeatedly"
	default:
		return "unknown"
	}
}

func worse(a, b Modality) Modality { return max(a, b) }

// enclosingElement returns the nearest HTML element ancestor on path
// and how control flow qualifies reaching the visited node from it.
// It returns nil inside attribute subtrees, comment bodies, and define
// bodies (a comment's text is never parsed as content, and a define's
// element context depends on its call sites).
func enclosingElement(path []htt.Node) (*htt.HTMLElementNode, Modality) {
	mod := ModalityAlways
	for i := len(path) - 1; i >= 0; i-- {
		switch p := path[i].(type) {
		case *htt.HTMLElementNode:
			return p, mod
		case *htt.RangeNode:
			mod = worse(mod, ModalityRepeatedly)
		case *htt.IfNode, *htt.WithNode:
			mod = worse(mod, ModalityConditionally)
		case *htt.HTMLAttributeNode, *htt.DefineNode, *htt.HTMLCommentNode:
			return nil, mod
		}
	}
	return nil, mod
}

// elementAncestors yields HTML element ancestors from nearest to
// outermost with cumulative modality, stopping where enclosingElement
// would (attributes, comment bodies, define bodies).
func elementAncestors(path []htt.Node) iter.Seq2[*htt.HTMLElementNode, Modality] {
	return func(yield func(*htt.HTMLElementNode, Modality) bool) {
		mod := ModalityAlways
		for i := len(path) - 1; i >= 0; i-- {
			switch p := path[i].(type) {
			case *htt.HTMLElementNode:
				if !yield(p, mod) {
					return
				}
			case *htt.RangeNode:
				mod = worse(mod, ModalityRepeatedly)
			case *htt.IfNode, *htt.WithNode:
				mod = worse(mod, ModalityConditionally)
			case *htt.HTMLAttributeNode, *htt.DefineNode, *htt.HTMLCommentNode:
				return
			}
		}
	}
}
