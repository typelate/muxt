// Package selectors implements a useful subset of the W3C Selectors
// specification (https://www.w3.org/TR/selectors-4/) over the dual
// HTML/template parse tree produced by github.com/typelate/htt.
//
// Because the htt tree is template source rather than a rendered DOM, a
// selector cannot always decide statically whether an element matches: an
// attribute value or the very presence of an element may depend on runtime
// data. Matching therefore yields a [Trit] — Yes, No, or Maybe — instead of a
// boolean.
package selectors

// Trit is the result of matching a selector against a node in a template tree.
// The htt tree describes template source, so some matches cannot be decided
// statically; those yield [TritMaybe]. The zero value is [TritNo].
//
// The three values are ordered No < Maybe < Yes as degrees of truth, so the
// Kleene logic operators reduce to min ([Trit.And]) and max ([Trit.Or]).
type Trit uint8

const (
	// TritNo means the node statically cannot match.
	TritNo Trit = iota
	// TritMaybe means the node may match depending on runtime data.
	TritMaybe
	// TritYes means the node statically matches.
	TritYes
)

func (t Trit) String() string {
	switch t {
	case TritNo:
		return "No"
	case TritMaybe:
		return "Maybe"
	case TritYes:
		return "Yes"
	default:
		return "Trit(invalid)"
	}
}

// Not is Kleene negation: it swaps Yes and No and leaves Maybe unchanged.
func (t Trit) Not() Trit {
	switch t {
	case TritYes:
		return TritNo
	case TritNo:
		return TritYes
	default:
		return TritMaybe
	}
}

// And is Kleene conjunction (the weaker of the two truth degrees).
func (t Trit) And(o Trit) Trit {
	if o < t {
		return o
	}
	return t
}

// Or is Kleene disjunction (the stronger of the two truth degrees).
func (t Trit) Or(o Trit) Trit {
	if o > t {
		return o
	}
	return t
}
