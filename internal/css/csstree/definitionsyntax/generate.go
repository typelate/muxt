// Ported from lib/definition-syntax/generate.js.

package definitionsyntax

import (
	"strconv"
	"strings"
)

// GenerateOptions configures Generate. The zero value generates the
// canonical form.
type GenerateOptions struct {
	// ForceBraces wraps every group in square brackets.
	ForceBraces bool
	// Compact omits optional whitespace.
	Compact bool
	// Decorate, when set, is called with the text generated for each node
	// and returns the text to use instead.
	Decorate func(str string, node Node) string
}

func noop(value string, _ Node) string {
	return value
}

func generateMultiplier(multiplier *Multiplier) string {
	min, max, comma := multiplier.Min, multiplier.Max, multiplier.Comma

	if min == 0 && max == Unbounded {
		if comma {
			return "#?"
		}
		return "*"
	}

	if min == 0 && max == 1 {
		return "?"
	}

	if min == 1 && max == Unbounded {
		if comma {
			return "#"
		}
		return "+"
	}

	if min == 1 && max == 1 {
		return ""
	}

	prefix := ""
	if comma {
		prefix = "#"
	}

	if min == max {
		return prefix + "{" + strconv.Itoa(min) + "}"
	}

	maxText := ""
	if max != Unbounded {
		maxText = strconv.Itoa(max)
	}
	return prefix + "{" + strconv.Itoa(min) + "," + maxText + "}"
}

// generateTypeOpts takes a *Range, the only node type upstream accepts.
func generateTypeOpts(node *Range) string {
	min := "-∞"
	if node.Min != nil {
		min = node.Min.String()
	}

	max := "∞"
	if node.Max != nil {
		max = node.Max.String()
	}

	return " [" + min + "," + max + "]"
}

func generateSequence(node *Group, decorate func(string, Node) string, forceBraces, compact bool) string {
	combinator := " " + node.Combinator + " "
	if node.Combinator == " " || compact {
		combinator = node.Combinator
	}

	terms := make([]string, 0, len(node.Terms))
	for _, term := range node.Terms {
		terms = append(terms, internalGenerate(term, decorate, forceBraces, compact))
	}
	result := strings.Join(terms, combinator)

	if node.Explicit || forceBraces {
		open := "[ "
		if compact || strings.HasPrefix(result, ",") {
			open = "["
		}

		closing := " ]"
		if compact {
			closing = "]"
		}

		return open + result + closing
	}

	return result
}

func internalGenerate(node Node, decorate func(string, Node) string, forceBraces, compact bool) string {
	var result string

	switch node := node.(type) {
	case *Group:
		result = generateSequence(node, decorate, forceBraces, compact)
		if node.DisallowEmpty {
			result += "!"
		}

	case *Multiplier:
		// return since node is a composition
		return internalGenerate(node.Term, decorate, forceBraces, compact) +
			decorate(generateMultiplier(node), node)

	case *Boolean:
		result = "<boolean-expr[" + internalGenerate(node.Term, decorate, forceBraces, compact) + "]>"

	case *Type:
		opts := ""
		if node.Opts != nil {
			opts = decorate(generateTypeOpts(node.Opts), node.Opts)
		}
		result = "<" + node.Name + opts + ">"

	case *Property:
		result = "<'" + node.Name + "'>"

	case *Keyword:
		result = node.Name

	case *AtKeyword:
		result = "@" + node.Name

	case *Function:
		result = node.Name + "("

	case *String:
		result = node.Value

	case *Token:
		result = node.Value

	case *Comma:
		result = ","

	default:
		// upstream throws "Unknown node type"; Node is sealed, so only a
		// nil node (or a *Range, which upstream also rejects) reaches here
		return ""
	}

	return decorate(result, node)
}

// Generate returns the definition syntax text for node. A nil node (or a
// *Range on its own) generates the empty string.
func Generate(node Node, options GenerateOptions) string {
	decorate := noop
	if options.Decorate != nil {
		decorate = options.Decorate
	}

	return internalGenerate(node, decorate, options.ForceBraces, options.Compact)
}
