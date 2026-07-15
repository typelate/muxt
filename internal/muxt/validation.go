package muxt

import (
	"cmp"
	"fmt"
	"go/types"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/dom/spec"
	"golang.org/x/net/html/atom"

	"github.com/typelate/muxt/internal/asteval"
)

// InputValidation is one request-value constraint parsed from a form field's
// <input> element attributes. The generate package renders each constraint as
// a guard statement in the handler.
type InputValidation interface{ inputValidation() }

// MinValidation is the min attribute of a numeric or temporal input; Min
// holds the attribute value already checked against the field's type.
type MinValidation struct {
	Name string
	Min  string
}

// MaxValidation is the max attribute of a numeric or temporal input; Max
// holds the attribute value already checked against the field's type.
type MaxValidation struct {
	Name string
	Max  string
}

// PatternValidation is the pattern attribute of a textual input.
type PatternValidation struct {
	Name    string
	Pattern *regexp.Regexp
}

// MinLengthValidation is the minlength attribute of an input.
type MinLengthValidation struct {
	Name      string
	MinLength int
}

// MaxLengthValidation is the maxlength attribute of an input.
type MaxLengthValidation struct {
	Name      string
	MaxLength int
}

func (MinValidation) inputValidation()       {}
func (MaxValidation) inputValidation()       {}
func (PatternValidation) inputValidation()   {}
func (MinLengthValidation) inputValidation() {}
func (MaxLengthValidation) inputValidation() {}

// ParseInputValidations parses the constraint attributes (min, max, pattern,
// minlength, maxlength) of a form field's <input> element. Attribute values
// are validated here — min and max must parse as the field's type tp — so
// resolution fails before any code generation begins.
func ParseInputValidations(name string, input spec.Element, tp types.Type) ([]InputValidation, error) {
	if tag := strings.ToLower(input.TagName()); tag != atom.Input.String() {
		return nil, fmt.Errorf("expected element to have tag <input> got <%s>", tag)
	}
	var result []InputValidation
	typeAttr := cmp.Or(input.GetAttribute("type"), "text")
	if slices.Contains([]string{
		"date", "month", "week", "time", "datetime-local", "number", "range",
	}, typeAttr) {
		if input.HasAttribute("min") {
			val := input.GetAttribute("min")
			if _, err := asteval.ParseWithType(val, tp); err != nil {
				return nil, err
			}
			result = append(result, MinValidation{Name: name, Min: val})
		}
		if input.HasAttribute("max") {
			val := input.GetAttribute("max")
			if _, err := asteval.ParseWithType(val, tp); err != nil {
				return nil, err
			}
			result = append(result, MaxValidation{Name: name, Max: val})
		}
	}
	if slices.Contains([]string{
		"text", "search", "url", "tel", "email", "password",
	}, typeAttr) && input.HasAttribute("pattern") {
		val := input.GetAttribute("pattern")
		exp, err := regexp.Compile(val)
		if err != nil {
			return nil, err
		}
		result = append(result, PatternValidation{Name: name, Pattern: exp})
	}
	var minL MinLengthValidation
	if val := input.GetAttribute("minlength"); val != "" {
		n, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("minlength must be an integer: %w", err)
		}
		if n < 0 {
			return nil, fmt.Errorf("minlength must not be negative")
		}
		minL = MinLengthValidation{Name: name, MinLength: n}
		result = append(result, minL)
	}
	if val := input.GetAttribute("maxlength"); val != "" {
		maxLength, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("maxlength must be an integer: %w", err)
		}
		if maxLength < 0 {
			return nil, fmt.Errorf("maxlength must not be negative")
		}
		if minL.MinLength != 0 {
			if minL.MinLength > maxLength {
				return nil, fmt.Errorf("maxlength (%d) must be greater than or equal to minlength (%d)", maxLength, minL.MinLength)
			}
		}
		result = append(result, MaxLengthValidation{Name: name, MaxLength: maxLength})
	}
	return result, nil
}
