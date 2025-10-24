package astgen

import (
	"go/ast"
	"go/token"
)

// IterateGenDecl returns an iterator over GenDecl nodes with the specified token type
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

// IterateValueSpecs returns an iterator over ValueSpec nodes in var declarations
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

// IterateFieldTypes returns an iterator over field types in a field list
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

// FieldIndex returns the name and type of the field at the given index
func FieldIndex(fields []*ast.Field, i int) (*ast.Ident, ast.Expr, bool) {
	n := 0
	for _, field := range fields {
		if len(field.Names) == 0 {
			if n != i {
				n++
				continue
			}
			return nil, field.Type, true
		}
		for _, name := range field.Names {
			if n != i {
				n++
				continue
			}
			return name, field.Type, true
		}
	}
	return nil, nil, false
}

// FindFieldWithName finds a field in a field list by name
func FindFieldWithName(list *ast.FieldList, name string) (*ast.Field, bool) {
	for _, field := range list.List {
		for _, ident := range field.Names {
			if ident.Name == name {
				return field, true
			}
		}
	}
	return nil, false
}

// CallError creates an error.Error() call expression
func CallError(errIdent string) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(errIdent),
			Sel: ast.NewIdent("Error"),
		},
		Args: []ast.Expr{},
	}
}