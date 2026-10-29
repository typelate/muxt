// Ported from lib/parser/sequence.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Recognizer is a parse scope (upstream config.scope[name], the
// `recognizer` of readSequence): it turns the tokens of a sequence into
// nodes.
type Recognizer struct {
	// GetNode parses the node at the current token, or returns nil
	// (upstream undefined) to end the sequence.
	GetNode func(p *Parser, context *SequenceContext) Node
	// OnWhiteSpace, when set, is called when whitespace precedes next
	// (nil at the end of the sequence).
	OnWhiteSpace func(p *Parser, next Node, children *List[Node], context *SequenceContext)
	// Functions maps lowercase function names to parsers of the function
	// arguments; Function uses them instead of reading a sequence
	// (upstream: any other key of the scope object, e.g. var, expression).
	Functions map[string]func(p *Parser, recognizer *Recognizer) *List[Node]
}

// SequenceContext is the context object ReadSequence passes to a
// Recognizer.
type SequenceContext struct {
	Recognizer *Recognizer
}

// ReadSequence reads nodes with recognizer until it returns nil, skipping
// comments and whitespace.
func (p *Parser) ReadSequence(recognizer *Recognizer) *List[Node] {
	children := p.CreateList()
	space := false
	context := &SequenceContext{
		Recognizer: recognizer,
	}

loop:
	for !p.EOF {
		switch p.TokenType {
		case tokenizer.Comment:
			p.Next()
			continue

		case tokenizer.WhiteSpace:
			space = true
			p.Next()
			continue
		}

		child := recognizer.GetNode(p, context)

		if child == nil {
			break loop
		}

		if space {
			if recognizer.OnWhiteSpace != nil {
				recognizer.OnWhiteSpace(p, child, children, context)
			}
			space = false
		}

		children.Push(child)
	}

	if space && recognizer.OnWhiteSpace != nil {
		recognizer.OnWhiteSpace(p, nil, children, context)
	}

	return children
}
