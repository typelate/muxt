package asteval

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/typelate/muxt/internal/asterr"
	"github.com/typelate/muxt/internal/astgen"
)

func StringLiteralExpression(wd string, set *token.FileSet, exp ast.Expr) (string, error) {
	arg, ok := exp.(*ast.BasicLit)
	if !ok || arg.Kind != token.STRING {
		return "", asterr.WrapWithFilename(wd, set, exp.Pos(), fmt.Errorf("expected string literal got %s", astgen.Format(exp)))
	}
	return strconv.Unquote(arg.Value)
}

func StringLiteralExpressionList(wd string, set *token.FileSet, list []ast.Expr) ([]string, error) {
	result := make([]string, 0, len(list))
	for _, a := range list {
		s, err := StringLiteralExpression(wd, set, a)
		if err != nil {
			return result, err
		}
		result = append(result, s)
	}
	return result, nil
}
