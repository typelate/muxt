// Package overlay holds muxt's template engine in a form that can replace
// the standard library's html/template under go build -overlay.
package overlay

import (
	"embed"
	"io/fs"
)

//go:generate go run ./internal/gen -o _engine

// engine is the copy internal/gen writes. The directory name starts with an
// underscore so the go command does not treat the copy, whose imports only
// resolve inside $GOROOT/src/html/template, as packages of this module.
//
//go:embed _engine
var engine embed.FS

// Engine returns the engine's source laid out as $GOROOT/src/html/template:
// the html/template facade at the root, and its dependencies under
// internal/engine, internal/lex, internal/xhtml and internal/xhtml/atom.
func Engine() fs.FS {
	sub, err := fs.Sub(engine, "_engine")
	if err != nil {
		panic(err) // unreachable: _engine is a valid path in engine
	}
	return sub
}
