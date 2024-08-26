package source

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
)

func IterateGenDecl(files []*ast.File, tok token.Token) func(func(*ast.File, *ast.GenDecl) bool) {
	return func(yield func(*ast.File, *ast.GenDecl) bool) {
		for _, file := range files {
			for _, decl := range file.Decls {
				d, ok := decl.(*ast.GenDecl)
				if !ok || d.Tok != tok {
					continue
				}
				if !yield(file, d) {
					return
				}
			}
		}
	}
}

func IterateValueSpecs(files []*ast.File) func(func(*ast.File, *ast.ValueSpec) bool) {
	return func(yield func(*ast.File, *ast.ValueSpec) bool) {
		for file, decl := range IterateGenDecl(files, token.VAR) {
			for _, s := range decl.Specs {
				if !yield(file, s.(*ast.ValueSpec)) {
					return
				}
			}
		}
	}
}

//func IterateTypes(files []*ast.File) func(func(*ast.File, *ast.TypeSpec) bool) {
//	return func(yield func(*ast.File, *ast.TypeSpec) bool) {
//		for _, file := range files {
//			for _, decl := range file.Decls {
//				spec, ok := decl.(*ast.GenDecl)
//				if !ok || spec.Tok != token.TYPE {
//					continue
//				}
//				for _, s := range spec.Specs {
//					t, ok := s.(*ast.TypeSpec)
//					if !ok {
//						continue
//					}
//					if !yield(file, t) {
//						return
//					}
//				}
//			}
//		}
//	}
//}

func IterateFunctions(files []*ast.File) func(func(*ast.File, *ast.FuncDecl) bool) {
	return func(yield func(*ast.File, *ast.FuncDecl) bool) {
		for _, file := range files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if !yield(file, fn) {
					return
				}
			}
		}
	}
}

//func IterateImports(files []*ast.File) func(func(*ast.File, *ast.ImportSpec) bool) {
//	return func(yield func(*ast.File, *ast.ImportSpec) bool) {
//		for _, file := range files {
//			for _, decl := range file.Decls {
//				genDecl, ok := decl.(*ast.GenDecl)
//				if !ok || genDecl.Tok != token.IMPORT {
//					continue
//				}
//				for _, s := range genDecl.Specs {
//					if !yield(file, s.(*ast.ImportSpec)) {
//						return
//					}
//				}
//			}
//		}
//	}
//}

func Format(node ast.Node) string {
	var buf strings.Builder
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("formatting error: %v", err)
	}
	return buf.String()
}

func evaluateStringLiteralExpressionList(wd string, set *token.FileSet, list []ast.Expr) ([]string, error) {
	result := make([]string, 0, len(list))
	for _, a := range list {
		s, err := evaluateStringLiteralExpression(wd, set, a)
		if err != nil {
			return result, err
		}
		result = append(result, s)
	}
	return result, nil
}

func evaluateStringLiteralExpression(wd string, set *token.FileSet, exp ast.Expr) (string, error) {
	arg, ok := exp.(*ast.BasicLit)
	if !ok || arg.Kind != token.STRING {
		return "", contextError(wd, set, exp.Pos(), fmt.Errorf("expected string literal got %s", Format(exp)))
	}
	return strconv.Unquote(arg.Value)
}

func IterateFieldTypes(list []*ast.Field) func(func(int, ast.Expr) bool) {
	return func(yield func(int, ast.Expr) bool) {
		i := 0
		for _, field := range list {
			if len(field.Names) == 0 {
				if !yield(i, field.Type) {
					return
				}
				i++
			} else {
				for range field.Names {
					if !yield(i, field.Type) {
						return
					}
					i++
				}
			}
		}
	}
}
