package source

import (
	"go/ast"
	"go/token"
	"slices"
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
				spec, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if !yield(file, spec) {
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

func IterateImports(files []*ast.File) func(func(*ast.File, *ast.ImportSpec) bool) {
	return func(yield func(*ast.File, *ast.ImportSpec) bool) {
		for _, file := range files {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.IMPORT {
					continue
				}
				for _, spec := range genDecl.Specs {
					importSpec, ok := spec.(*ast.ImportSpec)
					if !ok {
						continue
					}
					if !yield(file, importSpec) {
						return
					}
				}
			}
		}
	}
}

func SortImports(input []*ast.ImportSpec) []*ast.ImportSpec {
	slices.SortFunc(input, func(a, b *ast.ImportSpec) int { return strings.Compare(a.Path.Value, b.Path.Value) })
	return slices.CompactFunc(input, func(a, b *ast.ImportSpec) bool { return a.Path.Value == b.Path.Value })
}
