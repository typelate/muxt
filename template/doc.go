// Package template is muxt's template engine: a text/template parser that
// records source positions and parses HTML structure, and the execution
// engine for its trees. Package template/html builds an
// html/template-compatible API on it.
//
// The packages are public so that the coverage pre-pass of muxt
// test-template-mutations can require them from the module under test, and
// so that tools can use them. They are versioned with the muxt command:
// go get github.com/typelate/muxt/template@<version> gets the engine of
// muxt <version>. There is no stability promise beyond the command's own;
// any release may change their API.
//
// Derived in part from the Go standard library; see NOTICE.txt at the
// repository root.
package template
