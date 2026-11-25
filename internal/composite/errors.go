package composite

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/diagnostic"
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

// Diagnostic places the problem at the path, within it when Where is set. What
// is written is a fact, not an instruction, so it stays in the message.
func (e *ExistsError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: e.Where},
		Message: "already exists; nothing was written",
	}
}

// NoFileError reports a generate package run with no muxt.json between Dir and
// the module root, so there is no file to add the entry to.
type NoFileError struct {
	Dir string
}

// Error returns the diagnostic as one line of text.
func (e *NoFileError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic has no place, since there is no file, and offers the command that
// creates one as the fix.
func (e *NoFileError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Message: fmt.Sprintf("no muxt.json found at or above %s up to the module root", e.Dir),
		Fix:     "create one with: muxt generate config",
	}
}

// ParseError reports a muxt.json that is not valid JSON. Line and Col are the
// 1-based position of the byte the parser rejected, so a person can find it
// without counting bytes.
type ParseError struct {
	Path      string
	Line, Col int
	Err       error
}

// Error returns the diagnostic as one line of text.
func (e *ParseError) Error() string {
	return e.Diagnostic().String()
}

// Unwrap returns the JSON error, so errors.As finds the *json.SyntaxError.
func (e *ParseError) Unwrap() error {
	return e.Err
}

// Diagnostic places the problem at the byte the parser rejected.
func (e *ParseError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Line: e.Line, Column: e.Col},
		Message: fmt.Sprint(e.Err),
	}
}

// ShapeError reports a value in a muxt.json that is not an object where the
// file needs one. Line and Col are the 1-based position of the value. Where
// names it, such as "packages" or packages["example.com/m"], and Want is what
// it must be, finishing the sentence that starts with Where. Fix is the one
// edit that mends it, empty where the value could be several things.
type ShapeError struct {
	Path      string
	Line, Col int
	Where     string
	Want      string
	Fix       string
}

// Error returns the diagnostic as one line of text.
func (e *ShapeError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the value, and says what it must be.
func (e *ShapeError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Line: e.Line, Column: e.Col},
		Message: e.Where + " must be " + e.Want,
		Fix:     e.Fix,
	}
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

// Error returns the diagnostic as one line of text.
func (e *RepeatedKeyError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the second key, within the object that
// holds it.
func (e *RepeatedKeyError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Line: e.Line, Column: e.Col, Within: e.Where},
		Message: fmt.Sprintf("key %q is repeated", e.Key),
	}
}

// UnknownKeyError reports a key in a muxt.json that nothing reads: a key of an
// entry that no generate flag reads, or a top-level key other than
// package_defaults and packages. Where names the entry, such as
// packages["."], and is empty for a top-level key. Suggestion is the closest
// known key, empty when none is close, so a typo names its fix.
type UnknownKeyError struct {
	Path, Where, Key, Suggestion string
}

// Error returns the diagnostic as one line of text.
func (e *UnknownKeyError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the entry, or the file for a top-level key,
// and offers the suggestion as the fix.
func (e *UnknownKeyError) Diagnostic() diagnostic.Diagnostic {
	d := diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: e.Where},
		Message: fmt.Sprintf("unknown key %q", e.Key),
	}
	if e.Suggestion != "" {
		d.Fix = fmt.Sprintf("did you mean %q?", e.Suggestion)
	}
	return d
}

// KeyFormError reports a packages key that is not an import path in the clean
// form Go names a package by: a directory relative to the file, such as
// "workspace" or "./workspace/", which keys used to be, or an import path with
// a stray slash. ImportPath is the key to write instead, so two spellings of
// one package cannot both be keys.
type KeyFormError struct {
	Path, Key, ImportPath string
}

// Error returns the diagnostic as one line of text.
func (e *KeyFormError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file, and names the import path as the
// fix.
func (e *KeyFormError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("packages key %q is not an import path", e.Key),
		Fix:     fmt.Sprintf("write %q", e.ImportPath),
	}
}

// MissingEntryError reports a directory below a muxt.json that the file has no
// entry for, so a flagless muxt generate there has nothing to read. Key is the
// packages key the directory would need.
type MissingEntryError struct {
	Path, Key string
}

// Error returns the diagnostic as one line of text.
func (e *MissingEntryError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file, and says to add the entry as the
// fix.
func (e *MissingEntryError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("no packages entry %q", e.Key),
		Fix:     "add one to generate this directory",
	}
}

// MissingDirectoryError reports a muxt.json entry whose directory does not
// exist or is a file. Key is the packages key and Dir the key's absolute
// directory. IsFile is set when Dir exists and is not a directory, so the
// message does not claim a path that is there is missing.
type MissingDirectoryError struct {
	Path, Key, Dir string
	IsFile         bool
}

// Error returns the diagnostic as one line of text.
func (e *MissingDirectoryError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the entry of the key.
func (e *MissingDirectoryError) Diagnostic() diagnostic.Diagnostic {
	message := fmt.Sprintf("directory %s does not exist", e.Dir)
	if e.IsFile {
		message = fmt.Sprintf("%s is not a directory", e.Dir)
	}
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: fmt.Sprintf("packages[%q]", e.Key)},
		Message: message,
	}
}

// OutsideDirectoryError reports a muxt.json entry whose directory is a symbolic
// link that resolves outside the directory holding the file. Key is the
// packages key and Dir the key's absolute directory.
type OutsideDirectoryError struct {
	Path, Key, Dir string
}

// Error returns the diagnostic as one line of text.
func (e *OutsideDirectoryError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the entry of the key.
func (e *OutsideDirectoryError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: fmt.Sprintf("packages[%q]", e.Key)},
		Message: fmt.Sprintf("directory %s resolves outside the file's directory", e.Dir),
	}
}

// SameDirectoryError reports two packages keys whose directories are one
// directory once symbolic links are followed. Both would be generated, and
// the later entry's settings would win. Key is the later key, Other the
// earlier, and Dir the directory they share with its links followed.
type SameDirectoryError struct {
	Path, Key, Other, Dir string
}

// Error returns the diagnostic as one line of text.
func (e *SameDirectoryError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic puts both keys in the message, as Error does, and offers each as a
// related place, the earlier first.
func (e *SameDirectoryError) Diagnostic() diagnostic.Diagnostic {
	within := func(key string) diagnostic.Place {
		return diagnostic.Place{File: e.Path, Within: fmt.Sprintf("packages[%q]", key)}
	}
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("packages[%q] is the same directory as packages[%q], %s", e.Key, e.Other, e.Dir),
		Fix:     "keep one",
		Related: []diagnostic.Related{
			{Place: within(e.Other), Message: "the earlier key"},
			{Place: within(e.Key), Message: "the later key"},
		},
	}
}

// clashingEntries are the two entries under key of the file at path, the
// earlier first, as related places.
func clashingEntries(path, key string, first, second int) []diagnostic.Related {
	return []diagnostic.Related{
		{Place: diagnostic.Place{File: path, Within: SetWhere(key, first)}, Message: "the earlier entry"},
		{Place: diagnostic.Place{File: path, Within: SetWhere(key, second)}, Message: "the later entry"},
	}
}

// SetClashError reports two entries under one packages key that have one value
// for a member that must differ: output_file, since both would write one file,
// or output_routes_func, since generated files are owned by routes function and
// a run of one entry would delete the other's file. "Have", not "set": a
// default counts, and neither entry may have written the member. Member is the
// JSON member, Value what both resolve to, and First and Second the zero-based
// positions under Key, First being the earlier.
type SetClashError struct {
	Path, Key, Member, Value string
	First, Second            int
}

// Error returns the diagnostic as one line of text.
func (e *SetClashError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic names both entries in the message, as Error does, and offers each
// as a related place.
func (e *SetClashError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("%s and %s both have %s %q", SetWhere(e.Key, e.First), SetWhere(e.Key, e.Second), e.Member, e.Value),
		Fix:     "give each its own " + e.Member,
		Related: clashingEntries(e.Path, e.Key, e.First, e.Second),
	}
}

// RoutePathsSetsError reports a packages key with several entries in a file
// that has a route_paths key. The hub holds one namespace per package, so a
// package with two route sets would declare its namespace twice. Sets is how
// many entries Key holds.
type RoutePathsSetsError struct {
	Path, Key string
	Sets      int
}

// Error returns the diagnostic as one line of text.
func (e *RoutePathsSetsError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file, and offers the two ways out as the
// fix.
func (e *RoutePathsSetsError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("route_paths takes one entry per package, and packages[%q] has %d", e.Key, e.Sets),
		Fix:     "keep one entry or remove route_paths",
	}
}

// GeneratedFileClashError reports two entries under one packages key that
// would write one generated file under output_multiple_files: both read a
// template file, and the file's name comes from the template file alone, so the
// second would overwrite the first. File is the absolute path of the generated
// file, Path the muxt.json, and First and Second the zero-based positions under
// Key, First being the earlier.
type GeneratedFileClashError struct {
	Path, Key, File string
	First, Second   int
}

// Error returns the diagnostic as one line of text.
func (e *GeneratedFileClashError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the generated file, as Error does, and
// offers the muxt.json entries that would both write it as related places.
func (e *GeneratedFileClashError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.File},
		Message: fmt.Sprintf("%s and %s would both write it under output_multiple_files", SetWhere(e.Key, e.First), SetWhere(e.Key, e.Second)),
		Fix:     "give them different template files or turn output_multiple_files off",
		Related: clashingEntries(e.Path, e.Key, e.First, e.Second),
	}
}

// InvalidEntryError reports a muxt.json entry whose values a generate flag
// rejects, such as two frontends set at once. Err is the flag's own error, so
// the message matches the one the command line gives. Where names the entry.
type InvalidEntryError struct {
	Path, Where string
	Err         error
}

// Error returns the diagnostic as one line of text.
func (e *InvalidEntryError) Error() string {
	return e.Diagnostic().String()
}

// Unwrap returns the flag's error, so errors.Is and errors.As reach it.
func (e *InvalidEntryError) Unwrap() error {
	return e.Err
}

// Diagnostic places the problem at the entry, and says what the flag said.
func (e *InvalidEntryError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: e.Where},
		Message: fmt.Sprint(e.Err),
	}
}

// NoVariableError reports a muxt.json entry whose package does not declare the
// templates variable the entry names, or the default one. Path is the file's
// absolute path, Key the entry's packages key, Package the package's import
// path and Variable the name asked for. Set is the zero-based position of the
// entry under Key, so a key with several entries says which one to fix.
type NoVariableError struct {
	Path, Key, Package, Variable string
	Set                          int
}

// Error returns the diagnostic as one line of text.
func (e *NoVariableError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the entry, and offers the two fixes apart
// from what is wrong.
func (e *NoVariableError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: SetWhere(e.Key, e.Set)},
		Message: fmt.Sprintf("package %s declares no variable %q", e.Package, e.Variable),
		Fix:     "declare it, or remove the entry",
	}
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

// Error returns the diagnostic as one line of text.
func (e *ConfiguredDirectoryError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic has no place of its own, since the message names the file inline,
// and offers the file as the related place to configure the directory in.
func (e *ConfiguredDirectoryError) Diagnostic() diagnostic.Diagnostic {
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
	return diagnostic.Diagnostic{
		Message: msg,
		Related: []diagnostic.Related{{Place: diagnostic.Place{File: e.Path}, Message: "the file to configure the directory in"}},
	}
}

// DeprecatedKeyError reports a key in a muxt.json entry that is an old flag
// name. Replacement is the key to write instead. Where names the entry.
type DeprecatedKeyError struct {
	Path, Where, Key, Replacement string
}

// Error returns the diagnostic as one line of text.
func (e *DeprecatedKeyError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the entry, and names the replacement as the
// fix.
func (e *DeprecatedKeyError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: e.Where},
		Message: fmt.Sprintf("key %q is deprecated", e.Key),
		Fix:     fmt.Sprintf("write %q", e.Replacement),
	}
}

// DashedKeyError reports a key in a muxt.json entry written with dashes, such
// as "use-receiver-type", where the same key with underscores is a known key.
// Underscored is that key with underscores, the form to write instead. Where
// names the entry.
type DashedKeyError struct {
	Path, Where, Key, Underscored string
}

// Error returns the diagnostic as one line of text.
func (e *DashedKeyError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the entry, and names the underscore form as
// the fix.
func (e *DashedKeyError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: e.Where},
		Message: fmt.Sprintf("key %q uses dashes", e.Key),
		Fix:     fmt.Sprintf("write %q", e.Underscored),
	}
}

// OtherModuleError reports a packages key that is an import path outside the
// module holding the muxt.json. Module is the file's module path, the prefix
// every key must have.
type OtherModuleError struct {
	Path, Key, Module string
}

// Error returns the diagnostic as one line of text.
func (e *OtherModuleError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file.
func (e *OtherModuleError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("packages key %q is not in this module (%s)", e.Key, e.Module),
	}
}

// KeyOutsideError reports a packages key that names a package in the file's
// module but not at or below the directory holding the muxt.json, or a
// relative path such as "../other" or "/abs". A file
// checked into a repository must not make muxt generate write elsewhere on the
// machine.
type KeyOutsideError struct {
	Path, Key string
}

// Error returns the diagnostic as one line of text.
func (e *KeyOutsideError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file.
func (e *KeyOutsideError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("packages key %q is outside the file's directory", e.Key),
	}
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

// Error returns the diagnostic as one line of text.
func (e *NoModuleError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file, or at the directory the file was
// to be created in, whose File is that directory. The reason keys need a module
// is an explanation, not an instruction, so it stays in the message; what to run
// when creating is the fix.
func (e *NoModuleError) Diagnostic() diagnostic.Diagnostic {
	if e.Creating {
		return diagnostic.Diagnostic{
			Place:   diagnostic.Place{File: filepath.Dir(e.Path)},
			Message: "no go.mod found at or above this directory",
			Fix:     "run go mod init MODULE here first",
		}
	}
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: "no go.mod found at or above the file; package keys are import paths",
	}
}

// PackageNameError reports a NAME that muxt generate package cannot make a
// route package of. Reason finishes the sentence that starts with the name, and
// says what NAME is or what to do instead.
type PackageNameError struct {
	Name, Reason string
}

// Error returns the diagnostic as one line of text.
func (e *PackageNameError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic has no place: the name comes from the command line. The reason may
// say what to do instead, so the whole sentence stays in the message.
func (e *PackageNameError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Message: fmt.Sprintf("package name %q %s", e.Name, e.Reason)}
}

// NestedError reports two muxt.json files in one module with one below the
// other. A run at the outer file and a run at the inner file would generate
// different code for the same package, so a module keeps one file per path
// from a directory to the module root. Inner and Outer are absolute paths.
// Unwritten is set when Outer is the file muxt generate config would have
// written: there is no file to move entries into, and since packages keys are
// import paths the inner file means the same thing at Outer, so the fix is to
// move it.
type NestedError struct {
	Inner, Outer string
	Unwritten    bool
}

// Error returns both files and the fix on one line.
func (e *NestedError) Error() string {
	if e.Unwritten {
		return fmt.Sprintf("%s already configures a directory below; a module's muxt.json files do not nest: move it to %s", e.Inner, e.Outer)
	}
	return fmt.Sprintf("%s is below %s; a module's muxt.json files do not nest: move its entries into %s and delete it", e.Inner, e.Outer, e.Outer)
}

// Diagnostic places the problem at the inner file and offers the outer as the
// related place. What follows the semicolon explains before it instructs, so
// all of it stays in the message.
func (e *NestedError) Diagnostic() diagnostic.Diagnostic {
	message := fmt.Sprintf("is below %s; a module's muxt.json files do not nest: move its entries into %s and delete it", e.Outer, e.Outer)
	if e.Unwritten {
		message = fmt.Sprintf("already configures a directory below; a module's muxt.json files do not nest: move it to %s", e.Outer)
	}
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Inner},
		Message: message,
		Related: []diagnostic.Related{{Place: diagnostic.Place{File: e.Outer}, Message: "the file it nests below"}},
	}
}

// LockedError reports a muxt.json that another muxt run is rewriting: its lock
// file was still there after the wait. Path and Lock are absolute paths. The
// lock outlives a run only when the run was killed, so the message says to
// delete it then.
type LockedError struct {
	Path, Lock string
}

// Error returns the file, the lock and the fix on one line.
func (e *LockedError) Error() string {
	return fmt.Sprintf("%s is locked by %s: another muxt run is writing it; delete the lock file if none is", e.Path, e.Lock)
}

// Diagnostic places the problem at the file, and offers deleting the lock as
// the fix.
func (e *LockedError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("locked by %s: another muxt run is writing it", e.Lock),
		Fix:     "delete the lock file if none is",
	}
}

// AlreadyConfiguredError reports a muxt.json at or above the directory a
// command would write one into. A second file would nest below it, so the
// directory is already configured. Path is the found file's absolute path.
type AlreadyConfiguredError struct {
	Path string
}

// Error returns the file and the command that adds a package, on one line.
func (e *AlreadyConfiguredError) Error() string {
	return fmt.Sprintf("%s already configures this directory; add a package with: muxt generate package NAME", e.Path)
}

// Diagnostic places the problem at the found file, and names the command that
// adds a package as the fix.
func (e *AlreadyConfiguredError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: "this directory is already configured",
		Fix:     "add a package with: muxt generate package NAME",
	}
}

// EmptyPackagesError reports a flagless muxt generate in the directory of a
// muxt.json that lists no packages, so there is nothing to generate from the
// file and defaults must not stand in for it. Path is the file's absolute path.
type EmptyPackagesError struct {
	Path string
}

// Error returns the diagnostic as one line of text.
func (e *EmptyPackagesError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file, and names the command that adds a
// package as the fix.
func (e *EmptyPackagesError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: "packages is empty",
		Fix:     "add a package with: muxt generate package NAME",
	}
}

// KeyCaseError reports a packages key that names a directory of the module in
// a different letter case than the disk does. A case-insensitive file system
// opens the directory either way, so the key would configure the package
// twice. OnDisk is the key spelled as the directory is.
type KeyCaseError struct {
	Path, Key, OnDisk string
}

// Error returns the diagnostic as one line of text.
func (e *KeyCaseError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file.
func (e *KeyCaseError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: fmt.Sprintf("packages key %q is spelled %q on disk", e.Key, e.OnDisk),
		Fix:     "write that",
	}
}

// SeveralPackagesError reports a command that acts on one package, run with no
// flags in the directory of a muxt.json that lists packages but not that
// directory: the file's directory holds no package to act on, and guessing one
// would fail late. Path is the file's absolute path and Dirs the entries'
// directories relative to it, in key order. Task says what the command does to
// the package ("tests one package") and Where how to name one ("pass one of"),
// since a command that takes a directory argument and one that runs in the
// working directory give different advice.
type SeveralPackagesError struct {
	Path, Command, Task, Where string
	Dirs                       []string
}

// Error returns the diagnostic as one line of text.
func (e *SeveralPackagesError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the file, and offers the directories to
// choose from as the fix.
func (e *SeveralPackagesError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path},
		Message: e.Command + " " + e.Task,
		Fix:     e.Where + ": " + strings.Join(e.Dirs, " "),
	}
}

// ValueTypeError reports a key in a muxt.json entry whose value is not the
// JSON type the key takes. Want is the type it takes. Where names the entry.
type ValueTypeError struct {
	Path, Where, Key string
	Want             Kind
}

// Error returns the diagnostic as one line of text.
func (e *ValueTypeError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the entry.
func (e *ValueTypeError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: e.Where},
		Message: fmt.Sprintf("key %q takes %s", e.Key, e.Want.description()),
	}
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

// RouteValueError reports a value in the route_paths object that is not usable
// as written, such as an output_file that is not a Go file name in the
// directory holding the muxt.json. Where names the object, Key the member, Value
// the value as written, and Rule what the value must be.
type RouteValueError struct {
	Path, Where, Key, Value, Rule string
}

// Error returns the diagnostic as one line of text.
func (e *RouteValueError) Error() string {
	return e.Diagnostic().String()
}

// Diagnostic places the problem at the object that holds the member.
func (e *RouteValueError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{File: e.Path, Within: e.Where},
		Message: fmt.Sprintf("%s %q %s", e.Key, e.Value, e.Rule),
	}
}

// ValueError reports a value of a muxt.json key that its setting's parser
// rejects, such as a byte size that is not one. Key is the muxt.json key and
// Err is the parser's error.
type ValueError struct {
	Key string
	Err error
}

// Error returns the diagnostic as one line of text.
func (e *ValueError) Error() string {
	return e.Diagnostic().String()
}

// Unwrap returns the parser's error, so a caller can match what it was.
func (e *ValueError) Unwrap() error { return e.Err }

// Diagnostic places the problem at the key and says what the parser said.
func (e *ValueError) Diagnostic() diagnostic.Diagnostic {
	return diagnostic.Diagnostic{
		Place:   diagnostic.Place{Within: e.Key},
		Message: e.Err.Error(),
	}
}
