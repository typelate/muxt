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
