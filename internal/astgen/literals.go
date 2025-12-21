package astgen

import (
	"go/ast"
	"go/token"
	"strconv"
)

// Int creates an integer literal AST node
func Int(n int) *ast.BasicLit {
	return &ast.BasicLit{Value: strconv.Itoa(n), Kind: token.INT}
}

// String creates a string literal AST node
func String(s string) *ast.BasicLit {
	return &ast.BasicLit{Value: strconv.Quote(s), Kind: token.STRING}
}

// Bool creates a boolean identifier AST node
func Bool(b bool) *ast.Ident {
	if b {
		return ast.NewIdent("true")
	}
	return ast.NewIdent("false")
}

// Nil creates a nil identifier AST node
func Nil() *ast.Ident {
	return ast.NewIdent("nil")
}

// EmptyStructType creates an empty struct type AST node
func EmptyStructType() *ast.StructType {
	return &ast.StructType{Fields: &ast.FieldList{}}
}
