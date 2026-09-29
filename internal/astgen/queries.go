package astgen

import (
	"go/ast"
)

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
