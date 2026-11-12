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
