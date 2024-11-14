package source

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strings"

	"github.com/crhntr/dom/spec"
	"golang.org/x/net/html/atom"
)

type ValidationGenerator interface {
	GenerateValidation(imports *Imports, variable ast.Expr, handleError func(string) ast.Stmt) ast.Stmt
}

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
			_, err := ParseStringWithType(val, tp)
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
			_, err := ParseStringWithType(val, tp)
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
	return result, nil
}
