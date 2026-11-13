package composite

import "fmt"

// ExistsError reports a file or directory a command would create that is
// already there, so the command wrote nothing. Path is the name the message
// gives it, an absolute path an editor can open. Where, when set, names what
// in the file at Path exists, such as packages["example.com/m/a"].
type ExistsError struct {
	Path, Where string
}

// Error returns the name and the fact that nothing was written, on one line.
func (e *ExistsError) Error() string {
	if e.Where != "" {
		return fmt.Sprintf("%s: %s already exists; nothing was written", e.Path, e.Where)
	}
	return fmt.Sprintf("%s already exists; nothing was written", e.Path)
}

// NoFileError reports a generate package run with no muxt.json between Dir and
// the module root, so there is no file to add the entry to.
type NoFileError struct {
	Dir string
}

// Error returns the fix on one line.
func (e *NoFileError) Error() string {
	return "no muxt.json found; create one with: muxt generate config"
}

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

// KeyFormError reports a packages key that is not an import path in the clean
// form Go names a package by: a directory relative to the file, such as
// "workspace" or "./workspace/", which keys used to be, or an import path with
// a stray slash. ImportPath is the key to write instead, so two spellings of
// one package cannot both be keys.
type KeyFormError struct {
	Path, Key, ImportPath string
}

// Error returns the path, the key, and the import path to write on one line.
func (e *KeyFormError) Error() string {
	return fmt.Sprintf("%s: packages key %q is not an import path; write %q", e.Path, e.Key, e.ImportPath)
}

// MissingEntryError reports a directory below a muxt.json that the file has no
// entry for, so a flagless muxt generate there has nothing to read. Key is the
// packages key the directory would need.
type MissingEntryError struct {
	Path, Key string
}

// Error returns the path and the key to add on one line.
func (e *MissingEntryError) Error() string {
	return fmt.Sprintf("%s: no packages entry %q; add one to generate this directory", e.Path, e.Key)
}

// MissingDirectoryError reports a muxt.json entry whose directory does not
// exist or is a file. Key is the packages key and Dir the key's absolute
// directory.
type MissingDirectoryError struct {
	Path, Key, Dir string
}

// Error returns the path, the entry, and the directory on one line.
func (e *MissingDirectoryError) Error() string {
	return fmt.Sprintf("%s: packages[%q]: directory %s does not exist", e.Path, e.Key, e.Dir)
}

// OutsideDirectoryError reports a muxt.json entry whose directory is a symbolic
// link that resolves outside the directory holding the file. Key is the
// packages key and Dir the key's absolute directory.
type OutsideDirectoryError struct {
	Path, Key, Dir string
}

// Error returns the path, the entry, and the directory on one line.
func (e *OutsideDirectoryError) Error() string {
	return fmt.Sprintf("%s: packages[%q]: directory %s resolves outside the file's directory", e.Path, e.Key, e.Dir)
}

// InvalidEntryError reports a muxt.json entry whose values a generate flag
// rejects, such as two frontends set at once. Err is the flag's own error, so
// the message matches the one the command line gives. Where names the entry.
type InvalidEntryError struct {
	Path, Where string
	Err         error
}

// Error returns the path, the entry, and the flag's error on one line.
func (e *InvalidEntryError) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.Path, e.Where, e.Err)
}

// Unwrap returns the flag's error, so errors.Is and errors.As reach it.
func (e *InvalidEntryError) Unwrap() error {
	return e.Err
}

// ConfiguredDirectoryError reports generate flags given in a directory whose
// muxt.json entry already configures it. Path is the file's absolute
// path. Command is the word the person typed for the command that
// was run, such as "check", so the message is about what they ran. Flags are the
// flag names given, sorted, without "--".
type ConfiguredDirectoryError struct {
	Path    string
	Command string
	Flags   []string
}

// Error returns the command and the path the person should configure the
// directory in, on one line.
func (e *ConfiguredDirectoryError) Error() string {
	return fmt.Sprintf("configure %s with fields in %s instead of flags", e.Command, e.Path)
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

// DashedKeyError reports a key in a muxt.json entry written with dashes, such
// as "use-receiver-type", where the same key with underscores is a known key.
// Underscored is that key with underscores, the form to write instead. Where
// names the entry.
type DashedKeyError struct {
	Path, Where, Key, Underscored string
}

// Error returns the path, the entry, the key, and its underscore form on one
// line.
func (e *DashedKeyError) Error() string {
	return fmt.Sprintf("%s: %s: key %q is written with underscores; write %q", e.Path, e.Where, e.Key, e.Underscored)
}

// KeyOutsideError reports a packages key that names a package outside the
// directory holding the muxt.json: an import path of another module or above
// the file's directory, or a relative path such as "../other" or "/abs". A file
// checked into a repository must not make muxt generate write elsewhere on the
// machine.
type KeyOutsideError struct {
	Path, Key string
}

// Error returns the path and the key on one line.
func (e *KeyOutsideError) Error() string {
	return fmt.Sprintf("%s: packages key %q is outside the file's directory", e.Path, e.Key)
}

// NoModuleError reports a muxt.json with no go.mod at or above it. Its packages
// keys are import paths, which only a module path can complete, so the file
// cannot be read. Path is the file's absolute path.
type NoModuleError struct {
	Path string
}

// Error returns the path and the fact that keys are import paths on one line.
func (e *NoModuleError) Error() string {
	return fmt.Sprintf("%s: no go.mod found at or above the file; package keys are import paths", e.Path)
}

// PackageNameError reports a NAME that muxt generate package cannot make a
// route package of. Reason finishes the sentence that starts with the name, and
// says what NAME is or what to do instead.
type PackageNameError struct {
	Name, Reason string
}

// Error returns the name and the reason on one line.
func (e *PackageNameError) Error() string {
	return fmt.Sprintf("package name %q %s", e.Name, e.Reason)
}

// AlreadyConfiguredError reports a muxt.json at or above the directory a
// command would write one into. A second file would shadow it: the nearest file
// wins, so the directory would read as unconfigured. Path is the found file's
// absolute path.
type AlreadyConfiguredError struct {
	Path string
}

// Error returns the file and the command that adds a package, on one line.
func (e *AlreadyConfiguredError) Error() string {
	return fmt.Sprintf("%s already configures this directory; add a package with: muxt generate package NAME", e.Path)
}

// EmptyPackagesError reports a flagless muxt generate in the directory of a
// muxt.json that lists no packages, so there is nothing to generate from the
// file and defaults must not stand in for it. Path is the file's absolute path.
type EmptyPackagesError struct {
	Path string
}

// Error returns the file and the command that adds a package, on one line.
func (e *EmptyPackagesError) Error() string {
	return fmt.Sprintf("%s: packages is empty; add a package with: muxt generate package NAME", e.Path)
}
