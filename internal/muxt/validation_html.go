package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/dom/spec"
	"golang.org/x/net/html/atom"

	"github.com/typelate/muxt/internal/asteval"
)

func ParseInputValidations(name string, input spec.Element, tp types.Type) ([]ValidationGenerator, error) {
	if tag := strings.ToLower(input.TagName()); tag != atom.Input.String() {
		return nil, fmt.Errorf("expected element to have tag <input> got <%s>", tag)
	}
	var result []ValidationGenerator
	typeAttr := cmp.Or(input.GetAttribute("type"), "text")
	if slices.Contains([]string{
		"date", "month", "week", "time", "datetime-local", "number", "range",
	}, typeAttr) {
		if input.HasAttribute("min") {
			val := input.GetAttribute("min")
			_, err := asteval.ParseWithType(val, tp)
			if err != nil {
				return nil, err
			}
			result = append(result, MinValidation{
				Name:   name,
				MinExp: &ast.BasicLit{Value: val, Kind: token.INT},
			})
		}
		if input.HasAttribute("max") {
			val := input.GetAttribute("max")
			_, err := asteval.ParseWithType(val, tp)
			if err != nil {
				return nil, err
			}
			result = append(result, MaxValidation{
				Name:   name,
				MinExp: &ast.BasicLit{Value: val, Kind: token.INT},
			})
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
		result = append(result, PatternValidation{
			Name: name,
			Exp:  exp,
		})
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
		minL = MinLengthValidation{
			Name:      name,
			MinLength: n,
		}
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
		result = append(result, MaxLengthValidation{
			Name:      name,
			MaxLength: maxLength,
		})
	}
	return result, nil
}
