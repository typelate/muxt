package generate

import (
	"strings"
	"unicode"
)

func parsePatterns(input string) ([]string, error) {
	// todo: refactor to use strconv.QuotedPrefix
	var (
		patterns       []string
		currentPattern strings.Builder
		inQuote        = false
		quoteChar      rune
	)

	for _, r := range input {
		switch {
		case r == '"' || r == '`':
			if !inQuote {
				inQuote = true
				quoteChar = r
				continue
			}
			if r != quoteChar {
				currentPattern.WriteRune(r)
				continue
			}
			patterns = append(patterns, currentPattern.String())
			currentPattern.Reset()
			inQuote = false
		case unicode.IsSpace(r):
			if inQuote {
				currentPattern.WriteRune(r)
				continue
			}
			if currentPattern.Len() > 0 {
				patterns = append(patterns, currentPattern.String())
				currentPattern.Reset()
			}
		default:
			currentPattern.WriteRune(r)
		}
	}

	// Add any remaining pattern
	if currentPattern.Len() > 0 {
		patterns = append(patterns, currentPattern.String())
	}

	return patterns, nil
}
