// Package templatecover runs a package's tests with muxt's template engine
// in place of html/template and reads which template blocks they executed.
//
// It is the part of the coverage pre-pass of muxt test-template-mutations
// that does not depend on mutation: the go test flags the engine owns, the
// go command's process, and the profiles the test binaries write.
package templatecover
