package generate

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

type ValidationErrorBlock func(string) *ast.BlockStmt

// renderValidations renders the guard statements for the input constraints
// resolved by muxt.ResolveCall (muxt.ParseInputValidations).
func renderValidations(im astgen.ImportManager, variable ast.Expr, validations []muxt.InputValidation, handleError ValidationErrorBlock) []ast.Stmt {
	var statements []ast.Stmt
	for _, validation := range validations {
		statements = append(statements, renderValidation(im, variable, validation, handleError))
	}
	return statements
}

func renderValidation(im astgen.ImportManager, variable ast.Expr, validation muxt.InputValidation, handleError ValidationErrorBlock) ast.Stmt {
	switch val := validation.(type) {
	case muxt.MinValidation:
		return &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  variable,
				Op: token.LSS, // value < 13
				Y:  &ast.BasicLit{Value: val.Min, Kind: token.INT},
			},
			Body: handleError(fmt.Sprintf("%s must not be less than %s", val.Name, val.Min)),
		}
	case muxt.MaxValidation:
		return &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  variable,
				Op: token.GTR, // value > 13
				Y:  &ast.BasicLit{Value: val.Max, Kind: token.INT},
			},
			Body: handleError(fmt.Sprintf("%s must not be more than %s", val.Name, val.Max)),
		}
	case muxt.PatternValidation:
		return &ast.IfStmt{
			Cond: &ast.UnaryExpr{
				Op: token.NOT,
				X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   astgen.Call(im, "", "regexp", "MustCompile", astgen.String(val.Pattern.String())),
						Sel: ast.NewIdent("MatchString"),
					},
					Args: []ast.Expr{variable},
				},
			},
			Body: handleError(fmt.Sprintf("%s must match %q", val.Name, val.Pattern.String())),
		}
	case muxt.MinLengthValidation:
		return &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{variable}},
				Op: token.LSS,
				Y:  astgen.Int(val.MinLength),
			},
			Body: handleError(fmt.Sprintf("%s is too short (the min length is %d)", val.Name, val.MinLength)),
		}
	case muxt.MaxLengthValidation:
		return &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{variable}},
				Op: token.GTR,
				Y:  astgen.Int(val.MaxLength),
			},
			Body: handleError(fmt.Sprintf("%s is too long (the max length is %d)", val.Name, val.MaxLength)),
		}
	default:
		panic(fmt.Sprintf("unknown input validation type %T", validation))
	}
}
