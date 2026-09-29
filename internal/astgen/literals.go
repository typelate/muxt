package astgen

import (
	"go/ast"
	"go/token"
	"strconv"
)

func Int(n int) *ast.BasicLit {
	return &ast.BasicLit{Value: strconv.Itoa(n), Kind: token.INT}
}

func String(s string) *ast.BasicLit {
	return &ast.BasicLit{Value: strconv.Quote(s), Kind: token.STRING}
}

func Bool(b bool) *ast.Ident {
	if b {
		return ast.NewIdent("true")
	}
	return ast.NewIdent("false")
}

func Nil() *ast.Ident {
	return ast.NewIdent("nil")
}

func EmptyStructType() *ast.StructType {
	return &ast.StructType{Fields: &ast.FieldList{}}
}
