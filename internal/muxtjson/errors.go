package muxtjson

import "fmt"

// ParseError reports a muxt.json that is not valid JSON. Line and Col are the
// 1-based position of the byte the parser rejected, so a person can find it
// without counting bytes.
type ParseError struct {
	Path      string
	Line, Col int
	Err       error
}

// Error returns the path, the position, and the JSON error on one line.
func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %v", e.Path, e.Line, e.Col, e.Err)
}

// Unwrap returns the JSON error, so errors.As finds the *json.SyntaxError.
func (e *ParseError) Unwrap() error {
	return e.Err
}
