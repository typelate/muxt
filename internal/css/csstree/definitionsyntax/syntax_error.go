// Ported from lib/definition-syntax/SyntaxError.js and lib/utils/create-custom-error.js.

package definitionsyntax

import "strings"

// SyntaxError reports a malformed value definition syntax. Upstream builds it
// with createCustomError('SyntaxError', message); its name is the Go type.
type SyntaxError struct {
	// Message is the raw message followed by the input and a caret line
	// pointing at Offset.
	Message string
	// RawMessage is the message without the input excerpt.
	RawMessage string
	// Input is the whole source being parsed.
	Input string
	// Offset is the byte offset in Input where the error was detected.
	Offset int
}

func newSyntaxError(message, input string, offset int) *SyntaxError {
	// (offset || input.length): an offset of 0 points past the end, as upstream does
	caret := offset
	if caret == 0 {
		caret = len(input)
	}

	return &SyntaxError{
		Input:      input,
		Offset:     offset,
		RawMessage: message,
		Message: message + "\n" +
			"  " + input + "\n" +
			"--" + strings.Repeat("-", caret) + "^",
	}
}

// Error returns Message, the same text as upstream's error.message.
func (e *SyntaxError) Error() string {
	return e.Message
}
