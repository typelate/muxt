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

// PatternValidation is the pattern attribute of a textual input. Pattern is
// anchored at both ends, ^(?:pattern)$, since the attribute matches the
// whole value.
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
	inputType := cmp.Or(input.GetAttribute("type"), "text")
	var result []InputValidation
	if slices.Contains(boundedInputTypes, inputType) {
		bounds, err := parseBoundValidations(name, input, tp)
		if err != nil {
			return nil, err
		}
		result = append(result, bounds...)
	}
	if slices.Contains(patternInputTypes, inputType) {
		patterns, err := parsePatternValidations(name, input)
		if err != nil {
			return nil, err
		}
		result = append(result, patterns...)
	}
	lengths, err := parseLengthValidations(name, input)
	if err != nil {
		return nil, err
	}
	return append(result, lengths...), nil
}

// boundedInputTypes are the input types the min and max attributes apply to;
// patternInputTypes are the ones the pattern attribute applies to.
var (
	boundedInputTypes = []string{"date", "month", "week", "time", "datetime-local", "number", "range"}
	patternInputTypes = []string{"text", "search", "url", "tel", "email", "password"}
)

func parseBoundValidations(name string, input spec.Element, tp types.Type) ([]InputValidation, error) {
	var result []InputValidation
	if input.HasAttribute("min") {
		val := input.GetAttribute("min")
		if err := asteval.CheckParses(val, tp); err != nil {
			return nil, err
		}
		result = append(result, MinValidation{Name: name, Min: val})
	}
	if input.HasAttribute("max") {
		val := input.GetAttribute("max")
		if err := asteval.CheckParses(val, tp); err != nil {
			return nil, err
		}
		result = append(result, MaxValidation{Name: name, Max: val})
	}
	return result, nil
}

// parsePatternValidations reads the pattern attribute. As in HTML, the value
// must match the whole pattern, so the compiled expression is anchored at
// both ends; the pattern is compiled on its own first so a syntax error
// quotes what was written.
func parsePatternValidations(name string, input spec.Element) ([]InputValidation, error) {
	if !input.HasAttribute("pattern") {
		return nil, nil
	}
	written := input.GetAttribute("pattern")
	if _, err := regexp.Compile(written); err != nil {
		return nil, err
	}
	pattern, err := regexp.Compile(`^(?:` + written + `)$`)
	if err != nil {
		return nil, err
	}
	return []InputValidation{PatternValidation{Name: name, Pattern: pattern}}, nil
}

func parseLengthValidations(name string, input spec.Element) ([]InputValidation, error) {
	var result []InputValidation
	minLength, hasMin, err := lengthAttribute(input, "minlength")
	if err != nil {
		return nil, err
	}
	if hasMin {
		result = append(result, MinLengthValidation{Name: name, MinLength: minLength})
	}
	maxLength, hasMax, err := lengthAttribute(input, "maxlength")
	if err != nil {
		return nil, err
	}
	if hasMax {
		if hasMin && minLength > maxLength {
			return nil, fmt.Errorf("maxlength (%d) must be greater than or equal to minlength (%d)", maxLength, minLength)
		}
		result = append(result, MaxLengthValidation{Name: name, MaxLength: maxLength})
	}
	return result, nil
}

// lengthAttribute reads a non-negative integer attribute; an empty or absent
// one is not present.
func lengthAttribute(input spec.Element, attribute string) (int, bool, error) {
	val := input.GetAttribute(attribute)
	if val == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, false, fmt.Errorf("%s must be an integer: %w", attribute, err)
	}
	if n < 0 {
		return 0, false, fmt.Errorf("%s must not be negative", attribute)
	}
	return n, true, nil
}
