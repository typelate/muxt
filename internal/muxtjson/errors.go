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

// UnknownKeyError reports a key in a muxt.json entry that no generate flag
// reads. Where names the entry, such as packages["."].
type UnknownKeyError struct {
	Path, Where, Key string
}

// Error returns the path, the entry, and the key on one line.
func (e *UnknownKeyError) Error() string {
	return fmt.Sprintf("%s: %s: unknown key %q", e.Path, e.Where, e.Key)
}

// DeprecatedKeyError reports a key in a muxt.json entry that is an old flag
// name. Replacement is the key to write instead. Where names the entry.
type DeprecatedKeyError struct {
	Path, Where, Key, Replacement string
}

// Error returns the path, the entry, the key, and its replacement on one line.
func (e *DeprecatedKeyError) Error() string {
	return fmt.Sprintf("%s: %s: key %q is deprecated; write %q", e.Path, e.Where, e.Key, e.Replacement)
}
