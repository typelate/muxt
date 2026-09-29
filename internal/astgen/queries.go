package astgen

import (
	"go/ast"
)

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

func CallError(errIdent string) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(errIdent),
			Sel: ast.NewIdent("Error"),
		},
		Args: []ast.Expr{},
	}
}
