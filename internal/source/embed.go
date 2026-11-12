package source

import "go/ast"

// EmbedPackage is a package other than the one being analyzed, as far as a
// ParseFS call needs it: the exported embed.FS variables it declares and the
// files its //go:embed patterns matched. It is plain data built from one
// load; nothing in it needs the go command.
type EmbedPackage struct {
	// ImportPath is the package's import path.
	ImportPath string

	// ModulePath is the module the package is in, empty for the standard
	// library. It is how a package of another module is told apart.
	ModulePath string

	// Dir is the package directory, absolute. Embedded paths are relative to it.
	Dir string

	// Vars are the package-level var declarations of its Go files, parsed
	// with comments into the load's token.FileSet. They are the only syntax
	// kept: the //go:embed comment is on the declaration. Nil when the
	// package is not in the analyzed package's module, or embeds nothing:
	// those are the packages ParseFS cannot read, and they are not parsed.
	Vars []*ast.GenDecl

	// EmbedFiles are the files go list reports the package embeds, absolute,
	// so //go:embed exclusion rules (leading . and _, all:) are go's own.
	EmbedFiles []string
}
