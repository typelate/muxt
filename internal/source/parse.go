package source

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/crhntr/dom/spec"
)

func GenerateParseValueFromStringStatements(imports *Imports, tmp string, str, typeExp ast.Expr, errCheck func(expr ast.Expr) ast.Stmt, validations []ast.Stmt, assignment func(ast.Expr) ast.Stmt) ([]ast.Stmt, error) {
	switch tp := typeExp.(type) {
	case *ast.Ident:
		convert := func(exp ast.Expr) ast.Stmt {
			return assignment(&ast.CallExpr{
				Fun:  ast.NewIdent(tp.Name),
				Args: []ast.Expr{exp},
			})
		}
		switch tp.Name {
		default:
			return nil, fmt.Errorf("method param type %s not supported", Format(typeExp))
		case "bool":
			return parseBlock(tmp, imports.StrconvParseBoolCall(str), validations, errCheck, assignment), nil
		case "int":
			return parseBlock(tmp, imports.StrconvAtoiCall(str), validations, errCheck, assignment), nil
		case "int8":
			return parseBlock(tmp, imports.StrconvParseIntCall(str, 10, 8), validations, errCheck, convert), nil
		case "int16":
			return parseBlock(tmp, imports.StrconvParseIntCall(str, 10, 16), validations, errCheck, convert), nil
		case "int32":
			return parseBlock(tmp, imports.StrconvParseIntCall(str, 10, 32), validations, errCheck, convert), nil
		case "int64":
			return parseBlock(tmp, imports.StrconvParseIntCall(str, 10, 64), validations, errCheck, assignment), nil
		case "uint":
			return parseBlock(tmp, imports.StrconvParseUintCall(str, 10, 0), validations, errCheck, convert), nil
		case "uint8":
			return parseBlock(tmp, imports.StrconvParseUintCall(str, 10, 8), validations, errCheck, convert), nil
		case "uint16":
			return parseBlock(tmp, imports.StrconvParseUintCall(str, 10, 16), validations, errCheck, convert), nil
		case "uint32":
			return parseBlock(tmp, imports.StrconvParseUintCall(str, 10, 32), validations, errCheck, convert), nil
		case "uint64":
			return parseBlock(tmp, imports.StrconvParseUintCall(str, 10, 64), validations, errCheck, assignment), nil
		case "string":
			if len(validations) == 0 {
				assign := assignment(str)
				statements := slices.Concat(validations, []ast.Stmt{assign})
				return statements, nil
			}
			statements := slices.Concat([]ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(tmp)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{str},
			}}, validations, []ast.Stmt{assignment(ast.NewIdent(tmp))})
			return statements, nil
		}
	case *ast.SelectorExpr:
		if pkg, ok := tp.X.(*ast.Ident); ok {
			switch pkg.Name {
			case imports.Ident("time"):
				return parseBlock(tmp, imports.TimeParseCall(time.DateOnly, str), validations, errCheck, assignment), nil
			}
		}
	}
	return nil, fmt.Errorf("unsupported type: %s", Format(typeExp))
}

func GenerateValidations(imports *Imports, variable ast.Expr, variableType types.Type, inputQuery, inputName, responseIdent string, fragment spec.DocumentFragment) ([]ast.Stmt, error, bool) {
	input := fragment.QuerySelector(inputQuery)
	if input == nil {
		return nil, nil, false
	}

	validations, err := ParseInputValidations(inputName, input, variableType)
	if err != nil {
		return nil, err, true
	}

	var statements []ast.Stmt
	for _, validation := range validations {
		statements = append(statements, validation.GenerateValidation(imports, variable, func(message string) ast.Stmt {
			return &ast.ExprStmt{X: imports.HTTPErrorCall(ast.NewIdent(responseIdent), &ast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(message),
			}, http.StatusBadRequest)}
		}))
	}
	return statements, nil, true
}

type MinValidation struct {
	Name   string
	MinExp ast.Expr
}

func (val MinValidation) GenerateValidation(_ *Imports, variable ast.Expr, handleError func(string) ast.Stmt) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  variable,
			Op: token.LSS, // value < 13
			Y:  val.MinExp,
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				handleError(fmt.Sprintf("%s must not be less than %s", val.Name, Format(val.MinExp))),
				&ast.ReturnStmt{},
			},
		},
	}
}

type MaxValidation struct {
	Name   string
	MinExp ast.Expr
}

func (val MaxValidation) GenerateValidation(_ *Imports, variable ast.Expr, handleError func(string) ast.Stmt) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  variable,
			Op: token.GTR, // value > 13
			Y:  val.MinExp,
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				handleError(fmt.Sprintf("%s must not be more than %s", val.Name, Format(val.MinExp))),
				&ast.ReturnStmt{},
			},
		},
	}
}

type PatternValidation struct {
	Name string
	Exp  *regexp.Regexp
}

func (val PatternValidation) GenerateValidation(imports *Imports, variable ast.Expr, handleError func(string) ast.Stmt) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.UnaryExpr{
			Op: token.NOT,
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   imports.Call("", "regexp", "MustCompile", []ast.Expr{String(val.Exp.String())}),
					Sel: ast.NewIdent("MatchString"),
				},
				Args: []ast.Expr{variable},
			},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				handleError(fmt.Sprintf("%s must match %q", val.Name, val.Exp.String())),
				&ast.ReturnStmt{},
			},
		},
	}
}

func parseBlock(tmpIdent string, parseCall ast.Expr, validations []ast.Stmt, handleErr, handleResult func(out ast.Expr) ast.Stmt) []ast.Stmt {
	const errIdent = "err"
	parse := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(tmpIdent), ast.NewIdent(errIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{parseCall},
	}
	errCheck := ErrorCheckReturn(errIdent, handleErr(&ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(errIdent),
			Sel: ast.NewIdent("Error"),
		},
		Args: []ast.Expr{},
	}))
	block := &ast.BlockStmt{List: []ast.Stmt{parse, errCheck}}
	block.List = append(block.List, validations...)
	block.List = append(block.List, handleResult(ast.NewIdent(tmpIdent)))
	return block.List
}
