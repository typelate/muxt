package muxt_test

import (
	"go/types"
	"strconv"
	"strings"
	"testing"

	"github.com/typelate/dom"
	"github.com/typelate/dom/spec"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/typelate/muxt/internal/muxt"
)

func inputElement(t *testing.T, markup string) spec.Element {
	t.Helper()
	nodes, err := html.ParseFragment(strings.NewReader(markup), &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	element := dom.NewDocumentFragment(nodes).QuerySelector("[name=field]")
	if element == nil {
		t.Fatalf("%q has no element named field", markup)
	}
	return element
}

func TestParseInputValidations(t *testing.T) {
	intType := types.Typ[types.Int]
	stringType := types.Typ[types.String]
	for _, tt := range []struct {
		name    string
		markup  string
		tp      types.Type
		want    []string
		wantErr string
	}{
		{name: "no constraints", markup: `<input name="field">`, tp: stringType},
		{name: "number min", markup: `<input name="field" type="number" min="3">`, tp: intType, want: []string{"min 3"}},
		{name: "number max", markup: `<input name="field" type="number" max="9">`, tp: intType, want: []string{"max 9"}},
		{name: "range min and max", markup: `<input name="field" type="range" min="1" max="9">`, tp: intType, want: []string{"min 1", "max 9"}},
		{name: "min is ignored on a text input", markup: `<input name="field" type="text" min="x">`, tp: stringType},
		{name: "date min is checked against the field type", markup: `<input name="field" type="date" min="2020-01-01">`, tp: intType, wantErr: `parsing "2020-01-01": invalid syntax`},
		{name: "max out of range for the field type", markup: `<input name="field" type="number" max="300">`, tp: types.Typ[types.Uint8], wantErr: "value out of range"},
		{name: "min on an unsupported field type", markup: `<input name="field" type="number" min="1.5">`, tp: types.Typ[types.Float64], wantErr: "type float64 unknown"},
		{name: "month", markup: `<input name="field" type="month" min="3">`, tp: intType, want: []string{"min 3"}},
		{name: "week", markup: `<input name="field" type="week" min="3">`, tp: intType, want: []string{"min 3"}},
		{name: "time", markup: `<input name="field" type="time" min="3">`, tp: intType, want: []string{"min 3"}},
		{name: "datetime-local", markup: `<input name="field" type="datetime-local" min="3">`, tp: intType, want: []string{"min 3"}},
		{name: "pattern defaults to text", markup: `<input name="field" pattern="[a-z]+">`, tp: stringType, want: []string{"pattern [a-z]+"}},
		{name: "pattern on search", markup: `<input name="field" type="search" pattern="a">`, tp: stringType, want: []string{"pattern a"}},
		{name: "pattern on url", markup: `<input name="field" type="url" pattern="a">`, tp: stringType, want: []string{"pattern a"}},
		{name: "pattern on tel", markup: `<input name="field" type="tel" pattern="a">`, tp: stringType, want: []string{"pattern a"}},
		{name: "pattern on email", markup: `<input name="field" type="email" pattern="a">`, tp: stringType, want: []string{"pattern a"}},
		{name: "pattern on password", markup: `<input name="field" type="password" pattern="a">`, tp: stringType, want: []string{"pattern a"}},
		{name: "pattern is ignored on a number input", markup: `<input name="field" type="number" pattern="(">`, tp: intType},
		{name: "invalid pattern", markup: `<input name="field" pattern="(">`, tp: stringType, wantErr: "error parsing regexp"},
		{name: "minlength", markup: `<input name="field" minlength="2">`, tp: stringType, want: []string{"minlength 2"}},
		{name: "maxlength", markup: `<input name="field" maxlength="5">`, tp: stringType, want: []string{"maxlength 5"}},
		{name: "minlength and maxlength", markup: `<input name="field" minlength="2" maxlength="5">`, tp: stringType, want: []string{"minlength 2", "maxlength 5"}},
		{name: "equal lengths", markup: `<input name="field" minlength="5" maxlength="5">`, tp: stringType, want: []string{"minlength 5", "maxlength 5"}},
		{name: "maxlength zero", markup: `<input name="field" minlength="0" maxlength="0">`, tp: stringType, want: []string{"minlength 0", "maxlength 0"}},
		{name: "minlength not an integer", markup: `<input name="field" minlength="x">`, tp: stringType, wantErr: "minlength must be an integer: "},
		{name: "minlength negative", markup: `<input name="field" minlength="-1">`, tp: stringType, wantErr: "minlength must not be negative"},
		{name: "maxlength not an integer", markup: `<input name="field" maxlength="x">`, tp: stringType, wantErr: "maxlength must be an integer: "},
		{name: "maxlength negative", markup: `<input name="field" maxlength="-1">`, tp: stringType, wantErr: "maxlength must not be negative"},
		{name: "maxlength below minlength", markup: `<input name="field" minlength="5" maxlength="2">`, tp: stringType, wantErr: "maxlength (2) must be greater than or equal to minlength (5)"},
		{name: "the constraints of every kind in order", markup: `<input name="field" type="text" pattern="a" minlength="1" maxlength="2">`, tp: stringType, want: []string{"pattern a", "minlength 1", "maxlength 2"}},
		{name: "not an input", markup: `<textarea name="field"></textarea>`, tp: stringType, wantErr: "expected element to have tag <input> got <textarea>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := muxt.ParseInputValidations("field", inputElement(t, tt.markup), tt.tp)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseInputValidations(%s) error = %v, want it to contain %q", tt.markup, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseInputValidations(%s) error = %v", tt.markup, err)
			}
			if described := describeValidations(got); strings.Join(described, "; ") != strings.Join(tt.want, "; ") {
				t.Errorf("ParseInputValidations(%s) = %q, want %q", tt.markup, described, tt.want)
			}
		})
	}
}

func describeValidations(validations []muxt.InputValidation) []string {
	var described []string
	for _, validation := range validations {
		switch v := validation.(type) {
		case muxt.MinValidation:
			described = append(described, "min "+v.Min)
		case muxt.MaxValidation:
			described = append(described, "max "+v.Max)
		case muxt.PatternValidation:
			described = append(described, "pattern "+v.Pattern.String())
		case muxt.MinLengthValidation:
			described = append(described, "minlength "+strconv.Itoa(v.MinLength))
		case muxt.MaxLengthValidation:
			described = append(described, "maxlength "+strconv.Itoa(v.MaxLength))
		}
	}
	return described
}
