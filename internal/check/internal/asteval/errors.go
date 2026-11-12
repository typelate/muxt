package asteval

import (
	"fmt"
	"go/token"
	"path/filepath"
)

func wrapWithFilename(workingDirectory string, set *token.FileSet, pos token.Pos, err error) error {
	return fmt.Errorf("%s: %w", relativePosition(workingDirectory, set, pos), err)
}

// relativePosition is the position of pos with its file relative to
// workingDirectory, the form every error of this package prints.
func relativePosition(workingDirectory string, set *token.FileSet, pos token.Pos) token.Position {
	p := set.Position(pos)
	p.Filename, _ = filepath.Rel(workingDirectory, p.Filename)
	return p
}

// UnexportedSharedFSError is the error for a ParseFS argument that names an
// unexported variable of another package. Go rejects that reference too, but
// only with its own message, so muxt reports the rule and the fix first.
type UnexportedSharedFSError struct {
	// Position is the argument, relative to the package directory.
	Position token.Position

	// Package is the argument's qualifier, as written.
	Package string

	// Name is the unexported variable's name.
	Name string
}

// Error is one line: the position, what failed and how to fix it.
func (e UnexportedSharedFSError) Error() string {
	return fmt.Sprintf("%s: %s.%s is unexported; ParseFS reads only an exported embed.FS variable of another package", e.Position, e.Package, e.Name)
}

// OtherModuleError is the error for a ParseFS argument whose package is outside
// the analyzed package's module. Its files are not part of this module's build
// as the embed patterns were written, so muxt does not read them.
type OtherModuleError struct {
	// Position is the argument, relative to the package directory.
	Position token.Position

	// ImportPath is the argument's package.
	ImportPath string

	// ThisModule is the analyzed package's module.
	ThisModule string
}

// Error is one line: the position, what failed and how to fix it.
func (e OtherModuleError) Error() string {
	return fmt.Sprintf("%s: package %s is not in this module (%s); a shared embed.FS must come from a package of this module", e.Position, e.ImportPath, e.ThisModule)
}

// NotEmbedFSError is the error for a ParseFS argument that names an exported
// variable of another package whose type is not embed.FS. Only an embed.FS
// carries the files its //go:embed matched, so any other fs.FS is refused.
type NotEmbedFSError struct {
	// Position is the argument, relative to the package directory.
	Position token.Position

	// Package is the argument's qualifier, as written.
	Package string

	// Name is the variable's name.
	Name string

	// Type is the variable's type as go/types prints it, such as io/fs.FS.
	Type string
}

// Error is one line: the position, what failed and how to fix it.
func (e NotEmbedFSError) Error() string {
	return fmt.Sprintf("%s: %s.%s is a %s, not an embed.FS; ParseFS reads an exported embed.FS variable of another package", e.Position, e.Package, e.Name, e.Type)
}
