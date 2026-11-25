package composite_test

import (
	"errors"
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
	for _, tt := range []struct {
		name   string
		err    diagnosed
		want   string
		text   string
		reason string
	}{
		{
			name: "ParseError",
			err:  &composite.ParseError{Path: "/m/muxt.json", Line: 4, Col: 9, Err: errors.New("invalid character '}' looking for beginning of object key string")},
			want: "/m/muxt.json:4:9: invalid character '}' looking for beginning of object key string",
		},
		{
			name: "ShapeError",
			err:  &composite.ShapeError{Path: "/m/muxt.json", Line: 2, Col: 15, Where: `packages["example.com/m"]`, Want: "an object or an array of objects"},
			want: `/m/muxt.json:2:15: packages["example.com/m"] must be an object or an array of objects`,
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
			name: "ValueError",
			err:  &composite.ValueError{Key: "output_max_size", Err: errors.New("invalid byte size")},
			want: "output_max_size: invalid byte size",
		},
	} {
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
