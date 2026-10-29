// Ported from lib/tokenizer/names.js.

package tokenizer

// TokenNames maps a TokenType to its name in CSS Syntax Module Level 3.
// It is read-only.
var TokenNames = [...]string{
	"EOF-token",
	"ident-token",
	"function-token",
	"at-keyword-token",
	"hash-token",
	"string-token",
	"bad-string-token",
	"url-token",
	"bad-url-token",
	"delim-token",
	"number-token",
	"percentage-token",
	"dimension-token",
	"whitespace-token",
	"CDO-token",
	"CDC-token",
	"colon-token",
	"semicolon-token",
	"comma-token",
	"[-token",
	"]-token",
	"(-token",
	")-token",
	"{-token",
	"}-token",
	"comment-token",
}
