package composite

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

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

// Error returns the directory searched from and the fix on one line.
func (e *NoFileError) Error() string {
	return fmt.Sprintf("no muxt.json found at or above %s up to the module root; create one with: muxt generate config", e.Dir)
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

// ShapeError reports a value in a muxt.json that is not an object where the
// file needs one. Line and Col are the 1-based position of the value. Where
// names it, such as "packages" or packages["example.com/m"], and Want is what
// it must be, finishing the sentence that starts with Where.
type ShapeError struct {
	Path      string
	Line, Col int
	Where     string
	Want      string
}

// Error returns the path, the position, and what the value must be on one
// line.
func (e *ShapeError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s must be %s", e.Path, e.Line, e.Col, e.Where, e.Want)
}

// RepeatedKeyError reports a key written twice in one object of a muxt.json.
// The json package keeps the last and drops the first without a word, which
// would drop a whole set of entries when the key is packages. Line and Col are
// the 1-based position of the second key. Where names the object, such as
// "packages" or packages["example.com/m"], and is empty for the file's own
// keys.
type RepeatedKeyError struct {
	Path      string
	Line, Col int
	Where     string
	Key       string
}

// Error returns the path, the position, the object and the key on one line.
func (e *RepeatedKeyError) Error() string {
	if e.Where == "" {
		return fmt.Sprintf("%s:%d:%d: key %q is repeated", e.Path, e.Line, e.Col, e.Key)
	}
	return fmt.Sprintf("%s:%d:%d: %s: key %q is repeated", e.Path, e.Line, e.Col, e.Where, e.Key)
}

// UnknownKeyError reports a key in a muxt.json that nothing reads: a key of an
// entry that no generate flag reads, or a top-level key other than
// package_defaults and packages. Where names the entry, such as
// packages["."], and is empty for a top-level key. Suggestion is the closest
// known key, empty when none is close, so a typo names its fix.
type UnknownKeyError struct {
	Path, Where, Key, Suggestion string
}

// Error returns the path, the entry, the key, and the suggestion on one line.
func (e *UnknownKeyError) Error() string {
	msg := fmt.Sprintf("%s: unknown key %q", e.Path, e.Key)
	if e.Where != "" {
		msg = fmt.Sprintf("%s: %s: unknown key %q", e.Path, e.Where, e.Key)
	}
	if e.Suggestion != "" {
		msg += fmt.Sprintf("; did you mean %q?", e.Suggestion)
	}
	return msg
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
// directory. IsFile is set when Dir exists and is not a directory, so the
// message does not claim a path that is there is missing.
type MissingDirectoryError struct {
	Path, Key, Dir string
	IsFile         bool
}

// Error returns the path, the entry, and the directory on one line.
func (e *MissingDirectoryError) Error() string {
	if e.IsFile {
		return fmt.Sprintf("%s: packages[%q]: %s is not a directory", e.Path, e.Key, e.Dir)
	}
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

// SameDirectoryError reports two packages keys whose directories are one
// directory once symbolic links are followed. Both would be generated, and
// the later entry's settings would win. Key is the later key, Other the
// earlier, and Dir the directory they share with its links followed.
type SameDirectoryError struct {
	Path, Key, Other, Dir string
}

// Error returns the path, both keys and the directory on one line.
func (e *SameDirectoryError) Error() string {
	return fmt.Sprintf("%s: packages[%q] is the same directory as packages[%q], %s; keep one", e.Path, e.Key, e.Other, e.Dir)
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
// flag names given, sorted, without "--". Keys maps a flag name to the muxt.json
// key that replaces it; composite does not know the settings, so the caller
// says, and a deprecated spelling maps to its current key.
type ConfiguredDirectoryError struct {
	Path    string
	Command string
	Flags   []string
	Keys    map[string]string
}

// Error returns the command, the path the person should configure the
// directory in, and each flag given with its key, on one line.
func (e *ConfiguredDirectoryError) Error() string {
	msg := fmt.Sprintf("configure %s with fields in %s instead of flags", e.Command, e.Path)
	for i, flag := range slices.Sorted(slices.Values(e.Flags)) {
		sep := ", "
		if i == 0 {
			sep = ": "
		}
		msg += sep + "--" + flag
		if key, ok := e.Keys[flag]; ok {
			msg += " (" + key + ")"
		}
	}
	return msg
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
	return fmt.Sprintf("%s: %s: key %q uses dashes; write %q", e.Path, e.Where, e.Key, e.Underscored)
}

// OtherModuleError reports a packages key that is an import path outside the
// module holding the muxt.json. Module is the file's module path, the prefix
// every key must have.
type OtherModuleError struct {
	Path, Key, Module string
}

// Error returns the path, the key, and the module on one line.
func (e *OtherModuleError) Error() string {
	return fmt.Sprintf("%s: packages key %q is not in this module (%s)", e.Path, e.Key, e.Module)
}

// KeyOutsideError reports a packages key that names a package in the file's
// module but not at or below the directory holding the muxt.json, or a
// relative path such as "../other" or "/abs". A file
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
// cannot be read. Path is the file's absolute path. Creating is set when the
// file does not exist yet, so the message names the directory and the command
// to run, not a file the person never wrote.
type NoModuleError struct {
	Path     string
	Creating bool
}

// Error returns the path and the fact that keys are import paths on one line,
// or the directory and what to run when the file was to be created.
func (e *NoModuleError) Error() string {
	if e.Creating {
		return fmt.Sprintf("%s: no go.mod found at or above this directory; run go mod init MODULE here first", filepath.Dir(e.Path))
	}
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

// KeyCaseError reports a packages key that names a directory of the module in
// a different letter case than the disk does. A case-insensitive file system
// opens the directory either way, so the key would configure the package
// twice. OnDisk is the key spelled as the directory is.
type KeyCaseError struct {
	Path, Key, OnDisk string
}

// Error returns the path, the key, and the spelling on disk on one line.
func (e *KeyCaseError) Error() string {
	return fmt.Sprintf("%s: packages key %q is spelled %q on disk; write that", e.Path, e.Key, e.OnDisk)
}

// SeveralPackagesError reports a command that tests one package, run with no
// flags in the directory of a muxt.json that lists packages but not that
// directory: the file's directory holds no package to act on, and guessing one
// would fail late. Path is the file's absolute path and Dirs the entries'
// directories relative to it, in key order.
type SeveralPackagesError struct {
	Path, Command string
	Dirs          []string
}

// Error returns the file, the command and the directories to pass, on one line.
func (e *SeveralPackagesError) Error() string {
	return fmt.Sprintf("%s: %s tests one package; pass one of: %s", e.Path, e.Command, strings.Join(e.Dirs, " "))
}

// ValueTypeError reports a key in a muxt.json entry whose value is not the
// JSON type the key takes. Want is the type it takes. Where names the entry.
type ValueTypeError struct {
	Path, Where, Key string
	Want             Kind
}

// Error returns the path, the entry, the key, and the type it takes on one
// line.
func (e *ValueTypeError) Error() string {
	return fmt.Sprintf("%s: %s: key %q takes %s", e.Path, e.Where, e.Key, e.Want.description())
}

// description is the kind as a phrase: "a JSON boolean".
func (k Kind) description() string {
	switch k {
	case KindBool:
		return "a JSON boolean"
	case KindList:
		return "a JSON array of strings"
	default:
		return "a JSON string"
	}
}

// ValueError reports a value of a muxt.json key that its setting's parser
// rejects, such as a byte size that is not one. Key is the muxt.json key and
// Err is the parser's error.
type ValueError struct {
	Key string
	Err error
}

// Error returns the key and the parser's error on one line.
func (e *ValueError) Error() string { return e.Key + ": " + e.Err.Error() }

// Unwrap returns the parser's error, so a caller can match what it was.
func (e *ValueError) Unwrap() error { return e.Err }
