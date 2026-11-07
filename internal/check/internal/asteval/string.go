package asteval

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/typelate/muxt/internal/check/internal/astgen"
)

// maxIdentifierDepth bounds how many package-level identifiers are followed
// from one expression, so a cycle such as `const a = b; const b = a` ends.
const maxIdentifierDepth = 16

func StringLiteralExpression(wd string, set *token.FileSet, exp ast.Expr) (string, error) {
	arg, ok := exp.(*ast.BasicLit)
	if !ok || arg.Kind != token.STRING {
		return "", wrapWithFilename(wd, set, exp.Pos(), fmt.Errorf("expected string literal got %s", astgen.Format(exp)))
	}
	return strconv.Unquote(arg.Value)
}

// stringExpression reads a string literal, or a package-level const or var in
// files whose initializer is a string literal.
func stringExpression(wd string, set *token.FileSet, files []*ast.File, exp ast.Expr) (string, error) {
	resolved := resolvePackageLevel(files, exp)
	if lit, ok := resolved.(*ast.BasicLit); !ok || lit.Kind != token.STRING {
		return StringLiteralExpression(wd, set, exp)
	}
	return StringLiteralExpression(wd, set, resolved)
}

func StringLiteralExpressionList(wd string, set *token.FileSet, files []*ast.File, list []ast.Expr) ([]string, error) {
	result := make([]string, 0, len(list))
	for _, a := range list {
		s, err := stringExpression(wd, set, files, a)
		if err != nil {
			return result, err
		}
		result = append(result, s)
	}
	return result, nil
}

// resolvePackageLevel follows identifiers to the initializer of the
// package-level const or var they name. It returns exp unchanged when exp is
// not an identifier or the name has no initializer in files.
func resolvePackageLevel(files []*ast.File, exp ast.Expr) ast.Expr {
	for range maxIdentifierDepth {
		ident, ok := exp.(*ast.Ident)
		if !ok {
			return exp
		}
		next := packageLevelInitializer(files, ident.Name)
		if next == nil {
			return exp
		}
		exp = next
	}
	return exp
}

func packageLevelInitializer(files []*ast.File, name string) ast.Expr {
	for _, tok := range []token.Token{token.VAR, token.CONST} {
		for _, decl := range astgen.IterateGenDecl(files, tok) {
			for _, s := range decl.Specs {
				spec, ok := s.(*ast.ValueSpec)
				if !ok || len(spec.Values) != len(spec.Names) {
					continue
				}
				for i, n := range spec.Names {
					if n.Name == name {
						return spec.Values[i]
					}
				}
			}
		}
	}
	return nil
}
