package check

import "github.com/typelate/muxt/internal/check/internal/asteval"

// EmbedPatterns splits the text after //go:embed on one comment line into its
// patterns: space-separated, each optionally in double quotes or backquotes so
// that it can hold a space. It is the reading ParseFS calls get, offered so
// that code outside check reads a //go:embed line the same way.
func EmbedPatterns(directive string) []string {
	return asteval.ParseEmbedPatterns(directive)
}
