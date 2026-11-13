package asteval

import (
	"errors"
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
	// Position is the argument, with its file by absolute path.
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
	// Position is the argument, with its file by absolute path.
	Position token.Position

	// ImportPath is the argument's package.
	ImportPath string

	// OtherModule is the module the package is in, empty for the standard
	// library.
	OtherModule string

	// ThisModule is the analyzed package's module.
	ThisModule string
}

// Error is one line: the position, what failed and how to fix it.
func (e OtherModuleError) Error() string {
	if e.OtherModule == "" {
		return fmt.Sprintf("%s: package %s is in the standard library, which holds no embed.FS muxt can read; a shared embed.FS must come from a package of this module (%s)", e.Position, e.ImportPath, e.ThisModule)
	}
	return fmt.Sprintf("%s: package %s is in module %s, not in this module (%s); a shared embed.FS must come from a package of this module", e.Position, e.ImportPath, e.OtherModule, e.ThisModule)
}

// SharedPackageUnavailableError is the error for a ParseFS argument whose
// package is not among the packages this evaluation was given embedded files
// for. That is a fact about the run, not about the package's module: the
// package may well be in this module, so the message must not say it is not.
type SharedPackageUnavailableError struct {
	// Position is the argument, with its file by absolute path.
	Position token.Position

	// ImportPath is the argument's package.
	ImportPath string
}

// Error is one line: the position, what failed and how to fix it.
func (e SharedPackageUnavailableError) Error() string {
	return fmt.Sprintf("%s: muxt has no embedded files for package %s; run muxt on the module so its packages are loaded together", e.Position, e.ImportPath)
}

// NotEmbedFSError is the error for a ParseFS argument that names an exported
// variable of another package whose type is not embed.FS. Only an embed.FS
// carries the files its //go:embed matched, so any other fs.FS is refused.
type NotEmbedFSError struct {
	// Position is the argument, with its file by absolute path.
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
	return fmt.Sprintf("%s: %s.%s has type %s, not embed.FS; ParseFS reads an exported embed.FS variable of another package", e.Position, e.Package, e.Name, e.Type)
}

// NoEmbedDirectiveError is the error for a ParseFS argument that names an
// exported embed.FS variable of another package that has no //go:embed
// directive. The program builds, since an embed.FS with no directive is an
// empty file system, so without this the pattern would match nothing and the
// cause would be hard to see.
type NoEmbedDirectiveError struct {
	// Position is the argument, with its file by absolute path.
	Position token.Position

	// Package is the argument's qualifier, as written.
	Package string

	// Name is the variable's name.
	Name string
}

// Error is one line: the position, what failed and how to fix it.
func (e NoEmbedDirectiveError) Error() string {
	return fmt.Sprintf("%s: %s.%s has no //go:embed directive, so it holds no files; add one to its declaration", e.Position, e.Package, e.Name)
}

// SharedParseError is a parse error in a file of another package's embed.FS.
// The parser names a file by its base name, which reads as if the file were in
// the package that consumes it, so this names it by absolute path, the way
// type errors in the same file are reported.
type SharedParseError struct {
	// Filename is the file's absolute path.
	Filename string

	// Line is the line the parser reported.
	Line int

	// Err is the parse error. Unwrap reaches it, and the *template.ParseError
	// inside it.
	Err error
}

// Error is one line: the file and line, then what the parser said.
func (e SharedParseError) Error() string {
	cause := e.Err
	if inner := errors.Unwrap(cause); inner != nil {
		cause = inner
	}
	return fmt.Sprintf("%s:%d: %s", e.Filename, e.Line, cause)
}

// Unwrap returns the parse error.
func (e SharedParseError) Unwrap() error { return e.Err }
