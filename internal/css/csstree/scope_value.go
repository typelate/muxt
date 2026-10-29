// Ported from lib/syntax/scope/value.js.

package csstree

func isPlusMinusOperator(node Node) (*Operator, bool) {
	operator, ok := node.(*Operator)
	if !ok || operator == nil || operator.Value == "" {
		return nil, false
	}
	last := operator.Value[len(operator.Value)-1]
	return operator, last == '-' || last == '+'
}

func valueOnWhiteSpace(_ *Parser, next Node, children *List[Node], _ *SequenceContext) {
	if operator, ok := isPlusMinusOperator(next); ok {
		operator.Value = " " + operator.Value
	}
	if operator, ok := isPlusMinusOperator(children.Last()); ok {
		operator.Value += " "
	}
}

func valueScope() *Recognizer {
	return &Recognizer{
		GetNode:      defaultRecognizer,
		OnWhiteSpace: valueOnWhiteSpace,
		Functions: map[string]func(p *Parser, recognizer *Recognizer) *List[Node]{
			"expression": expressionFunction,
			"var":        varFunction,
		},
	}
}
