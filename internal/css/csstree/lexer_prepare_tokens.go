// Ported from lib/lexer/prepare-tokens.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// matchToken is a token the matcher consumes (upstream {type, value,
// node}). node is the AST node that generated the token, nil for tokens of
// a string value and for whitespace the generator added.
type matchToken struct {
	typ   tokenizer.TokenType
	value string
	node  Node
}

// matchInput is a value to match: an AST (node, nil for upstream
// null/undefined) or CSS text (css when isCSS). Upstream takes either a
// string or a node in the same argument.
type matchInput struct {
	node  Node
	css   string
	isCSS bool
}

func nodeInput(node Node) matchInput {
	return matchInput{node: node}
}

func cssInput(css string) matchInput {
	return matchInput{css: css, isCSS: true}
}

// truthy reports whether upstream's value would be truthy: a non-empty
// string or a node.
func (v matchInput) truthy() bool {
	if v.isCSS {
		return v.css != ""
	}

	return v.node != nil
}

// astToTokens is the generator decorator collecting the emitted tokens
// with the node being generated (upstream astToTokens.decorator).
func astToTokens(tokens *[]matchToken) func(handlers GenerateHandlers) GenerateHandlers {
	return func(handlers GenerateHandlers) GenerateHandlers {
		var curNode Node
		node := handlers.Node

		handlers.Node = func(h *GenerateHandlers, n Node) {
			tmp := curNode
			curNode = n
			node(h, n)
			curNode = tmp
		}
		handlers.Emit = func(_ *GenerateHandlers, value string, tokenType tokenizer.TokenType, auto bool) {
			token := matchToken{
				typ:   tokenType,
				value: value,
			}
			if !auto {
				token.node = curNode
			}
			*tokens = append(*tokens, token)
		}
		handlers.Result = func(*GenerateHandlers) string {
			return ""
		}

		return handlers
	}
}

func stringToTokens(str string) []matchToken {
	var tokens []matchToken

	tokenizer.Tokenize(str, func(tokenType tokenizer.TokenType, start, end int) {
		tokens = append(tokens, matchToken{
			typ:   tokenType,
			value: str[start:end],
		})
	})

	return tokens
}

// prepareTokens returns the tokens of value: a string is tokenized, an AST
// is generated with syntax. Generating fails for a node of an unknown type
// (upstream throws).
func prepareTokens(value matchInput, syntax *Syntax) ([]matchToken, error) {
	if value.isCSS {
		return stringToTokens(value.css), nil
	}

	var tokens []matchToken
	if _, err := syntax.Generate(value.node, GenerateOptions{Decorator: astToTokens(&tokens)}); err != nil {
		return nil, err
	}

	return tokens, nil
}
