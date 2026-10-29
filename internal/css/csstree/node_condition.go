// Ported from lib/syntax/node/Condition.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

func likelyFeatureToken(tokenType tokenizer.TokenType) bool {
	switch tokenType {
	case tokenizer.Colon, tokenizer.RightParenthesis, tokenizer.EOF:
		return true
	}
	return false
}

// Condition is a media, container or supports condition.
type Condition struct {
	Loc      *Location
	Kind     string
	Children *List[Node]
}

func (n *Condition) Type() string        { return "Condition" }
func (n *Condition) Location() *Location { return n.Loc }

func (n *Condition) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"kind", n.Kind},
		jsonField{"children", n.Children},
	)
}

func conditionNodeType() NodeConfig {
	return NodeConfig{
		Name: "Condition",
		Structure: []StructureField{
			{Name: "kind", Types: []FieldType{stringField()}},
			{Name: "children", Types: []FieldType{listField(
				"Identifier",
				"Feature",
				"FeatureFunction",
				"FeatureRange",
				"SupportsDeclaration",
			)}},
		},
		Generate:        generateCondition,
		Walk:            walkCondition,
		Clone:           cloneCondition,
		FromPlainObject: fromPlainObjectCondition,
	}
}

func walkCondition(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*Condition)
	return n.Children != nil && walkList(n.Children)
}

func cloneCondition(node Node, clone func(Node) Node) Node {
	c := *node.(*Condition)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectCondition(r *PlainObjectReader, object map[string]any) Node {
	return &Condition{
		Loc:      r.Location(object),
		Kind:     r.String(object, "kind"),
		Children: r.List(object, "children"),
	}
}

func featureOrRange(p *Parser, kind string) Node {
	if p.LookupTypeNonSC(1) == tokenizer.Ident &&
		likelyFeatureToken(p.LookupTypeNonSC(2)) {
		return p.Feature(kind)
	}

	return p.FeatureRange(kind)
}

// conditionParentheses returns the parser of a parenthesized term of a
// condition of kind, or nil.
func conditionParentheses(kind string) func(p *Parser, kind string) Node {
	switch kind {
	case "media", "container":
		return featureOrRange
	case "supports":
		return func(p *Parser, _ string) Node {
			return p.SupportsDeclaration()
		}
	}
	return nil
}

// Condition parses a condition of kind ("media", "container" or
// "supports"; "" means "media").
//
// The condition follows the grammar of Media Queries 4, CSS Conditional 3
// and CSS Containment 3: not and a single term, or terms joined by and,
// or terms joined by or (upstream accepts any sequence of identifiers and
// terms).
// https://drafts.csswg.org/mediaqueries-4/#media
func (p *Parser) Condition(kind string) *Condition {
	return p.condition(kind, true)
}

// condition parses a condition of kind; allowOr false parses a
// <media-condition-without-or>.
func (p *Parser) condition(kind string, allowOr bool) *Condition {
	if kind == "" {
		kind = "media"
	}
	children := p.CreateList()
	// the keyword that joins the terms: "not", "and", "or" or "" before
	// the first keyword
	keyword := ""
	expectTerm := true

scan:
	for !p.EOF {
		switch p.TokenType {
		case tokenizer.Comment, tokenizer.WhiteSpace:
			p.Next()
			continue

		case tokenizer.Ident:
			keyword = conditionKeyword(p, keyword, children.IsEmpty(), expectTerm, allowOr)
			expectTerm = true
			children.Push(p.Identifier())

		case tokenizer.LeftParenthesis:
			if !expectTerm {
				conditionTermError(p, keyword, allowOr)
			}
			expectTerm = false

			term := p.ParseWithFallback(
				func() Node {
					parentheses := conditionParentheses(kind)
					if parentheses == nil {
						// upstream: parentheses[kind] is not a function
						p.Error("Unknown condition kind `"+kind+"`", -1)
					}
					return parentheses(p, kind)
				},
				func() Node { return nil },
			)

			if term == nil {
				term = p.ParseWithFallback(
					func() Node {
						p.Eat(tokenizer.LeftParenthesis)
						res := p.Condition(kind)
						p.Eat(tokenizer.RightParenthesis)
						return res
					},
					func() Node {
						return p.GeneralEnclosed(kind)
					},
				)
			}

			children.Push(term)

		case tokenizer.Function:
			if !expectTerm {
				conditionTermError(p, keyword, allowOr)
			}
			expectTerm = false

			term := p.ParseWithFallback(
				func() Node { return p.FeatureFunction(kind) },
				func() Node { return nil },
			)

			if term == nil {
				term = p.GeneralEnclosed(kind)
			}

			children.Push(term)

		default:
			break scan
		}
	}

	if children.IsEmpty() {
		p.Error("Condition is expected", -1)
	}

	if expectTerm {
		p.Error("Parenthesis or function is expected", -1)
	}

	return &Condition{
		Loc:      p.GetLocationFromList(children),
		Kind:     kind,
		Children: children,
	}
}

// conditionKeyword checks the identifier at the cursor against the
// condition grammar and returns the keyword that joins the terms from now
// on. keyword is the keyword so far, first reports that no term or
// keyword precedes the identifier and expectTerm that a term is due.
func conditionKeyword(p *Parser, keyword string, first, expectTerm, allowOr bool) string {
	if expectTerm {
		// not <term> is a whole condition
		if first && p.LookupValue(0, "not") {
			return "not"
		}
		p.Error("Parenthesis or function is expected", -1)
	}

	switch {
	case keyword == "not":
		p.Error("", -1)
	case p.LookupValue(0, "and") && keyword != "or":
		return "and"
	case p.LookupValue(0, "or") && keyword != "and" && allowOr:
		return "or"
	}

	conditionTermError(p, keyword, allowOr)
	return "" // unreachable: conditionTermError panics
}

// conditionTermError reports a syntax error where the keyword that joins
// the terms of a condition, or the end of the condition, is due.
func conditionTermError(p *Parser, keyword string, allowOr bool) {
	switch {
	case keyword == "not":
		p.Error("", -1)
	case keyword == "" && allowOr:
		p.Error(`Identifier "and" or "or" is expected`, -1)
	case keyword == "":
		p.Error(`Identifier "and" is expected`, -1)
	default:
		p.Error(`Identifier "`+keyword+`" is expected`, -1)
	}
}

func generateCondition(g *Generator, node Node) {
	n := node.(*Condition)
	n.Children.ForEach(func(child Node, _ *ListItem[Node], _ *List[Node]) {
		if child.Type() == "Condition" {
			g.Token(tokenizer.LeftParenthesis, "(")
			g.Node(child)
			g.Token(tokenizer.RightParenthesis, ")")
		} else {
			g.Node(child)
		}
	})
}
