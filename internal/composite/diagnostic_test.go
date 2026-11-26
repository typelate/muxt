package composite_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/diagnostic"
)

// diagnosed is what every error type of the package that has been given a
// diagnostic offers.
type diagnosed interface {
	error
	Diagnostic() diagnostic.Diagnostic
}

// TestDiagnosticText states, for every error type of the package, that the
// one-line text of its diagnostic is its message. A row with a reason is a type
// whose Error() the text cannot reproduce byte for byte, so Error() stays as it
// was written: want is that message, text the diagnostic's.
func TestDiagnosticText(t *testing.T) {
	for _, tt := range diagnosticRows() {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error(), "Error()")
			if tt.reason == "" {
				assert.Equal(t, tt.want, tt.err.Diagnostic().String(), "diagnostic text")
				return
			}
			assert.Equal(t, tt.text, tt.err.Diagnostic().String(), "diagnostic text")
			assert.NotEqual(t, tt.want, tt.text, "a row with a reason differs from its message")
		})
	}
}

// diagnosticRow is an error built with representative values, the message it
// returns and the text of its diagnostic.
type diagnosticRow struct {
	name   string
	err    diagnosed
	want   string
	text   string
	reason string
}

// diagnosticRows lists every error type of the package.
func diagnosticRows() []diagnosticRow {
	return []diagnosticRow{
		{
			name: "ParseError",
			err:  &composite.ParseError{Path: "/m/muxt.json", Line: 4, Col: 9, Err: errors.New("invalid character '}' looking for beginning of object key string")},
			want: "/m/muxt.json:4:9: invalid character '}' looking for beginning of object key string",
		},
		{
			name: "ShapeError",
			err:  &composite.ShapeError{Path: "/m/muxt.json", Line: 2, Col: 15, Where: `packages["example.com/m"]`, Want: "an array of JSON objects", Fix: "wrap the entry in [ ]"},
			want: `/m/muxt.json:2:15: packages["example.com/m"] must be an array of JSON objects; wrap the entry in [ ]`,
		},
		{
			name: "RepeatedKeyError in an object",
			err:  &composite.RepeatedKeyError{Path: "/m/muxt.json", Line: 5, Col: 3, Where: "packages", Key: "example.com/m"},
			want: `/m/muxt.json:5:3: packages: key "example.com/m" is repeated`,
		},
		{
			name: "RepeatedKeyError at the top of the file",
			err:  &composite.RepeatedKeyError{Path: "/m/muxt.json", Line: 5, Col: 3, Key: "packages"},
			want: `/m/muxt.json:5:3: key "packages" is repeated`,
		},
		{
			name: "UnknownKeyError in an entry with a suggestion",
			err:  &composite.UnknownKeyError{Path: "/m/muxt.json", Where: `packages["."]`, Key: "output_fle", Suggestion: "output_file"},
			want: `/m/muxt.json: packages["."]: unknown key "output_fle"; did you mean "output_file"?`,
		},
		{
			name: "UnknownKeyError at the top without one",
			err:  &composite.UnknownKeyError{Path: "/m/muxt.json", Key: "pakages"},
			want: `/m/muxt.json: unknown key "pakages"`,
		},
		{
			name: "MissingDirectoryError for a missing directory",
			err:  &composite.MissingDirectoryError{Path: "/m/muxt.json", Key: "example.com/m/a", Dir: "/m/a"},
			want: `/m/muxt.json: packages["example.com/m/a"]: directory /m/a does not exist`,
		},
		{
			name: "MissingDirectoryError for a file",
			err:  &composite.MissingDirectoryError{Path: "/m/muxt.json", Key: "example.com/m/a", Dir: "/m/a", IsFile: true},
			want: `/m/muxt.json: packages["example.com/m/a"]: /m/a is not a directory`,
		},
		{
			name: "OutsideDirectoryError",
			err:  &composite.OutsideDirectoryError{Path: "/m/muxt.json", Key: "example.com/m/a", Dir: "/m/a"},
			want: `/m/muxt.json: packages["example.com/m/a"]: directory /m/a resolves outside the file's directory`,
		},
		{
			name: "InvalidEntryError",
			err:  &composite.InvalidEntryError{Path: "/m/muxt.json", Where: `packages["."][0]`, Err: errors.New("flags output-htmx and output-datastar are mutually exclusive")},
			want: `/m/muxt.json: packages["."][0]: flags output-htmx and output-datastar are mutually exclusive`,
		},
		{
			name: "NoVariableError",
			err:  &composite.NoVariableError{Path: "/m/muxt.json", Key: "example.com/m/a", Set: 1, Package: "example.com/m/a", Variable: "templates"},
			want: `/m/muxt.json: packages["example.com/m/a"][1]: package example.com/m/a declares no variable "templates"; declare it, or remove the entry`,
		},
		{
			name: "DeprecatedKeyError",
			err:  &composite.DeprecatedKeyError{Path: "/m/muxt.json", Where: `packages["."]`, Key: "templates_variable", Replacement: "use_templates_variable"},
			want: `/m/muxt.json: packages["."]: key "templates_variable" is deprecated; write "use_templates_variable"`,
		},
		{
			name: "DashedKeyError",
			err:  &composite.DashedKeyError{Path: "/m/muxt.json", Where: `packages["."]`, Key: "use-receiver-type", Underscored: "use_receiver_type"},
			want: `/m/muxt.json: packages["."]: key "use-receiver-type" uses dashes; write "use_receiver_type"`,
		},
		{
			name: "ValueTypeError",
			err:  &composite.ValueTypeError{Path: "/m/muxt.json", Where: `packages["."]`, Key: "output_htmx", Want: composite.KindBool},
			want: `/m/muxt.json: packages["."]: key "output_htmx" takes a JSON boolean`,
		},
		{
			name: "RouteValueError",
			err:  &composite.RouteValueError{Path: "/m/muxt.json", Where: "route_paths", Key: "output_file", Value: "../x.go", Rule: "must be a Go file name in this directory"},
			want: `/m/muxt.json: route_paths: output_file "../x.go" must be a Go file name in this directory`,
		},
		{
			name: "SameDirectoryError",
			err:  &composite.SameDirectoryError{Path: "/m/muxt.json", Key: "example.com/m/b", Other: "example.com/m/a", Dir: "/m/a"},
			want: `/m/muxt.json: packages["example.com/m/b"] is the same directory as packages["example.com/m/a"], /m/a; keep one`,
		},
		{
			name: "SetClashError",
			err:  &composite.SetClashError{Path: "/m/muxt.json", Key: "example.com/m", Member: "output_file", Value: "x.go", First: 0, Second: 1},
			want: `/m/muxt.json: packages["example.com/m"][0] and packages["example.com/m"][1] both have output_file "x.go"; give each its own output_file`,
		},
		{
			name: "GeneratedFileClashError",
			err:  &composite.GeneratedFileClashError{Path: "/m/muxt.json", Key: "example.com/m", File: "/m/a_routes_gen.go", First: 0, Second: 1},
			want: `/m/a_routes_gen.go: packages["example.com/m"][0] and packages["example.com/m"][1] would both write it under output_multiple_files; give them different template files or turn output_multiple_files off`,
		},
		{
			name: "NoFileError",
			err:  &composite.NoFileError{Dir: "/m/a"},
			want: "no muxt.json found at or above /m/a up to the module root; create one with: muxt generate config",
		},
		{
			name: "KeyFormError",
			err:  &composite.KeyFormError{Path: "/m/muxt.json", Key: "./a/", ImportPath: "example.com/m/a"},
			want: `/m/muxt.json: packages key "./a/" is not an import path; write "example.com/m/a"`,
		},
		{
			name: "MissingEntryError",
			err:  &composite.MissingEntryError{Path: "/m/muxt.json", Key: "example.com/m/a"},
			want: `/m/muxt.json: no packages entry "example.com/m/a"; add one to generate this directory`,
		},
		{
			name: "RoutePathsSetsError",
			err:  &composite.RoutePathsSetsError{Path: "/m/muxt.json", Key: "example.com/m/a", Sets: 2},
			want: `/m/muxt.json: route_paths takes one entry per package, and packages["example.com/m/a"] has 2; keep one entry or remove route_paths`,
		},
		{
			name: "ConfiguredDirectoryError",
			err:  &composite.ConfiguredDirectoryError{Path: "/m/muxt.json", Command: "check", Flags: []string{"x"}, Keys: map[string]string{"x": "y"}},
			want: "configure check with fields in /m/muxt.json instead of flags: --x (y)",
		},
		{
			name: "OtherModuleError",
			err:  &composite.OtherModuleError{Path: "/m/muxt.json", Key: "example.org/x", Module: "example.com/m"},
			want: `/m/muxt.json: packages key "example.org/x" is not in this module (example.com/m)`,
		},
		{
			name: "KeyOutsideError",
			err:  &composite.KeyOutsideError{Path: "/m/muxt.json", Key: "../other"},
			want: `/m/muxt.json: packages key "../other" is outside the file's directory`,
		},
		{
			name: "NoModuleError for a file",
			err:  &composite.NoModuleError{Path: "/m/muxt.json"},
			want: "/m/muxt.json: no go.mod found at or above the file; package keys are import paths",
		},
		{
			name: "NoModuleError for a file to create",
			err:  &composite.NoModuleError{Path: "/m/muxt.json", Creating: true},
			want: "/m: no go.mod found at or above this directory; run go mod init MODULE here first",
		},
		{
			name: "PackageNameError",
			err:  &composite.PackageNameError{Name: "Bad Name", Reason: "is not a Go identifier"},
			want: `package name "Bad Name" is not a Go identifier`,
		},
		{
			name: "EmptyPackagesError",
			err:  &composite.EmptyPackagesError{Path: "/m/muxt.json"},
			want: "/m/muxt.json: packages is empty; add a package with: muxt generate package NAME",
		},
		{
			name: "KeyCaseError",
			err:  &composite.KeyCaseError{Path: "/m/muxt.json", Key: "example.com/m/Abc", OnDisk: "example.com/m/abc"},
			want: `/m/muxt.json: packages key "example.com/m/Abc" is spelled "example.com/m/abc" on disk; write that`,
		},
		{
			name: "SeveralPackagesError",
			err:  &composite.SeveralPackagesError{Path: "/m/muxt.json", Command: "check", Task: "checks one package", Where: "pass one of", Dirs: []string{"a", "b"}},
			want: "/m/muxt.json: check checks one package; pass one of: a b",
		},
		{
			name:   "ExistsError in no entry",
			err:    &composite.ExistsError{Path: "/m/muxt.json"},
			want:   "/m/muxt.json already exists; nothing was written",
			text:   "/m/muxt.json: already exists; nothing was written",
			reason: "the message continues the path without a colon, and a place is always followed by one",
		},
		{
			name:   "ExistsError in an entry",
			err:    &composite.ExistsError{Path: "/m/muxt.json", Where: `packages["example.com/m/a"]`},
			want:   `/m/muxt.json: packages["example.com/m/a"] already exists; nothing was written`,
			text:   `/m/muxt.json: packages["example.com/m/a"]: already exists; nothing was written`,
			reason: "the message continues the entry without a colon, and a place is always followed by one",
		},
		{
			name:   "NestedError",
			err:    &composite.NestedError{Inner: "/m/a/muxt.json", Outer: "/m/muxt.json"},
			want:   "/m/a/muxt.json is below /m/muxt.json; a module's muxt.json files do not nest: move its entries into /m/muxt.json and delete it",
			text:   "/m/a/muxt.json: is below /m/muxt.json; a module's muxt.json files do not nest: move its entries into /m/muxt.json and delete it",
			reason: "the message continues the path without a colon, and a place is always followed by one",
		},
		{
			name:   "NestedError for a file not written yet",
			err:    &composite.NestedError{Inner: "/m/a/muxt.json", Outer: "/m/muxt.json", Unwritten: true},
			want:   "/m/a/muxt.json already configures a directory below; a module's muxt.json files do not nest: move it to /m/muxt.json",
			text:   "/m/a/muxt.json: already configures a directory below; a module's muxt.json files do not nest: move it to /m/muxt.json",
			reason: "the message continues the path without a colon, and a place is always followed by one",
		},
		{
			name:   "LockedError",
			err:    &composite.LockedError{Path: "/m/muxt.json", Lock: "/m/muxt.json.lock"},
			want:   "/m/muxt.json is locked by /m/muxt.json.lock: another muxt run is writing it; delete the lock file if none is",
			text:   "/m/muxt.json: locked by /m/muxt.json.lock: another muxt run is writing it; delete the lock file if none is",
			reason: "the message continues the path without a colon, and a place is always followed by one",
		},
		{
			name:   "AlreadyConfiguredError",
			err:    &composite.AlreadyConfiguredError{Path: "/m/muxt.json"},
			want:   "/m/muxt.json already configures this directory; add a package with: muxt generate package NAME",
			text:   "/m/muxt.json: already configures this directory; add a package with: muxt generate package NAME",
			reason: "the message continues the path without a colon, and a place is always followed by one",
		},
		{
			name: "ValueError",
			err:  &composite.ValueError{Key: "output_max_size", Err: errors.New("invalid byte size")},
			want: "output_max_size: invalid byte size",
		},
	}
}

// TestDiagnosticPlaceOfShapeAndParseErrors states that an error the parser
// found in the file places its diagnostic at the file, line and column an
// editor opens.
func TestDiagnosticPlaceOfShapeAndParseErrors(t *testing.T) {
	const path = "/m/muxt.json"
	want := diagnostic.Place{File: path, Line: 4, Column: 9}
	for _, tt := range []struct {
		name string
		err  diagnosed
		want diagnostic.Place
	}{
		{name: "ParseError", err: &composite.ParseError{Path: path, Line: 4, Col: 9, Err: errors.New("invalid character")}, want: want},
		{name: "ShapeError", err: &composite.ShapeError{Path: path, Line: 4, Col: 9, Where: "packages", Want: "an object"}, want: want},
		{
			name: "RepeatedKeyError keeps its object within the file",
			err:  &composite.RepeatedKeyError{Path: path, Line: 4, Col: 9, Where: "packages", Key: "a"},
			want: diagnostic.Place{File: path, Line: 4, Column: 9, Within: "packages"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Diagnostic().Place, "place")
		})
	}
}

// TestDiagnosticPlaceWithinEntry states that an error about one entry of the
// file places its diagnostic at that entry, written as the file's own
// notation, so the place is not part of the message.
func TestDiagnosticPlaceWithinEntry(t *testing.T) {
	const path = "/m/muxt.json"
	for _, tt := range []struct {
		name string
		err  diagnosed
		want diagnostic.Place
	}{
		{
			name: "an entry of a key with several",
			err:  &composite.NoVariableError{Path: path, Key: "example.com/m/a", Set: 1, Package: "example.com/m/a", Variable: "templates"},
			want: diagnostic.Place{File: path, Within: `packages["example.com/m/a"][1]`},
		},
		{
			name: "the entry of a key",
			err:  &composite.MissingDirectoryError{Path: path, Key: "example.com/m/a", Dir: "/m/a"},
			want: diagnostic.Place{File: path, Within: `packages["example.com/m/a"]`},
		},
		{
			name: "a top-level key is in no entry",
			err:  &composite.UnknownKeyError{Path: path, Key: "pakages"},
			want: diagnostic.Place{File: path},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Diagnostic().Place, "place")
		})
	}
}

// TestDiagnosticRelatedEntriesOfAClash states that an error naming two entries
// of the file offers each as a related place, the earlier first, so an editor
// can show both.
func TestDiagnosticRelatedEntriesOfAClash(t *testing.T) {
	const path = "/m/muxt.json"
	for _, tt := range []struct {
		name string
		err  diagnosed
		want []diagnostic.Related
	}{
		{
			name: "two positions under one key",
			err:  &composite.SetClashError{Path: path, Key: "example.com/m", Member: "output_file", Value: "x.go", First: 0, Second: 2},
			want: []diagnostic.Related{
				{Place: diagnostic.Place{File: path, Within: `packages["example.com/m"][0]`}, Message: "the earlier entry"},
				{Place: diagnostic.Place{File: path, Within: `packages["example.com/m"][2]`}, Message: "the later entry"},
			},
		},
		{
			name: "two entries that would write one file",
			err:  &composite.GeneratedFileClashError{Path: path, Key: "example.com/m", File: "/m/a_routes_gen.go", First: 0, Second: 1},
			want: []diagnostic.Related{
				{Place: diagnostic.Place{File: path, Within: `packages["example.com/m"][0]`}, Message: "the earlier entry"},
				{Place: diagnostic.Place{File: path, Within: `packages["example.com/m"][1]`}, Message: "the later entry"},
			},
		},
		{
			name: "two keys of one directory",
			err:  &composite.SameDirectoryError{Path: path, Key: "example.com/m/b", Other: "example.com/m/a", Dir: "/m/a"},
			want: []diagnostic.Related{
				{Place: diagnostic.Place{File: path, Within: `packages["example.com/m/a"]`}, Message: "the earlier key"},
				{Place: diagnostic.Place{File: path, Within: `packages["example.com/m/b"]`}, Message: "the later key"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Diagnostic().Related, "related")
		})
	}
}

// TestDiagnosticFixIsNotMessage states that what an error tells the person to
// do is the diagnostic's Fix, and what is wrong is its Message, where the
// message says what to do after its last "; ". A message that explains why
// before it instructs keeps all of that in Message.
func TestDiagnosticFixIsNotMessage(t *testing.T) {
	const path = "/m/muxt.json"
	for _, tt := range []struct {
		name        string
		err         diagnosed
		wantMessage string
		wantFix     string
	}{
		{
			name:        "a command to run",
			err:         &composite.EmptyPackagesError{Path: path},
			wantMessage: "packages is empty",
			wantFix:     "add a package with: muxt generate package NAME",
		},
		{
			name:        "a key to write",
			err:         &composite.KeyFormError{Path: path, Key: "./a/", ImportPath: "example.com/m/a"},
			wantMessage: `packages key "./a/" is not an import path`,
			wantFix:     `write "example.com/m/a"`,
		},
		{
			name:        "a file with no place",
			err:         &composite.NoFileError{Dir: "/m/a"},
			wantMessage: "no muxt.json found at or above /m/a up to the module root",
			wantFix:     "create one with: muxt generate config",
		},
		{
			name:        "directories to choose from",
			err:         &composite.SeveralPackagesError{Path: path, Command: "check", Task: "checks one package", Where: "pass one of", Dirs: []string{"a", "b"}},
			wantMessage: "check checks one package",
			wantFix:     "pass one of: a b",
		},
		{
			name:        "the words of Error() stay, with the file as the place",
			err:         &composite.AlreadyConfiguredError{Path: path},
			wantMessage: "already configures this directory",
			wantFix:     "add a package with: muxt generate package NAME",
		},
		{
			name:        "an explanation before the instruction stays whole",
			err:         &composite.NestedError{Inner: "/m/a/muxt.json", Outer: "/m/muxt.json"},
			wantMessage: "is below /m/muxt.json; a module's muxt.json files do not nest: move its entries into /m/muxt.json and delete it",
		},
		{
			name:        "a fact with no fix",
			err:         &composite.ExistsError{Path: path},
			wantMessage: "already exists; nothing was written",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.err.Diagnostic()
			assert.Equal(t, tt.wantMessage, d.Message, "message")
			assert.Equal(t, tt.wantFix, d.Fix, "fix")
		})
	}
}

// TestDiagnosticTableNamesEveryErrorType states that no error type of the
// package is left out of TestDiagnosticText: a new one must be given a row, and
// a reason when its Error() cannot be its diagnostic's text.
func TestDiagnosticTableNamesEveryErrorType(t *testing.T) {
	fset := token.NewFileSet()
	names, err := fs.Glob(os.DirFS("."), "*.go")
	if err != nil {
		t.Fatalf("listing the package's files: %v", err)
	}
	var declared []string
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, isStruct := ts.Type.(*ast.StructType); isStruct && ts.Name.IsExported() && strings.HasSuffix(ts.Name.Name, "Error") {
					declared = append(declared, ts.Name.Name)
				}
			}
		}
	}
	var listed []string
	for _, row := range diagnosticRows() {
		listed = append(listed, strings.TrimPrefix(fmt.Sprintf("%T", row.err), "*composite."))
	}
	slices.Sort(declared)
	slices.Sort(listed)
	assert.Equal(t, declared, slices.Compact(listed), "error types of the package, and those in the table")
}

// TestDiagnosticTextWithEmptyParts pins the text of an error about an entry
// when its path, place or key is empty, so a later change to what a missing
// part prints is seen.
func TestDiagnosticTextWithEmptyParts(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  diagnosed
		want string
	}{
		{
			name: "InvalidEntryError without a place",
			err:  &composite.InvalidEntryError{Path: "/m/muxt.json", Err: errors.New("flags output-htmx and output-datastar are mutually exclusive")},
			want: "/m/muxt.json: flags output-htmx and output-datastar are mutually exclusive",
		},
		{
			name: "InvalidEntryError without a path",
			err:  &composite.InvalidEntryError{Where: `packages["."][0]`, Err: errors.New("flags output-htmx and output-datastar are mutually exclusive")},
			want: `packages["."][0]: flags output-htmx and output-datastar are mutually exclusive`,
		},
		{
			name: "InvalidEntryError without a path or a place",
			err:  &composite.InvalidEntryError{Err: errors.New("flags output-htmx and output-datastar are mutually exclusive")},
			want: "flags output-htmx and output-datastar are mutually exclusive",
		},
		{
			name: "NoVariableError without a key",
			err:  &composite.NoVariableError{Path: "/m/muxt.json", Package: "example.com/m", Variable: "templates"},
			want: `/m/muxt.json: packages[""][0]: package example.com/m declares no variable "templates"; declare it, or remove the entry`,
		},
		{
			name: "NoVariableError without a path",
			err:  &composite.NoVariableError{Key: "example.com/m", Package: "example.com/m", Variable: "templates"},
			want: `packages["example.com/m"][0]: package example.com/m declares no variable "templates"; declare it, or remove the entry`,
		},
		{
			name: "MissingEntryError without a key",
			err:  &composite.MissingEntryError{Path: "/m/muxt.json"},
			want: `/m/muxt.json: no packages entry ""; add one to generate this directory`,
		},
		{
			name: "MissingEntryError without a path",
			err:  &composite.MissingEntryError{Key: "example.com/m/a"},
			want: `no packages entry "example.com/m/a"; add one to generate this directory`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Diagnostic().String(), "diagnostic text")
			assert.Equal(t, tt.want, tt.err.Error(), "Error()")
		})
	}
}
