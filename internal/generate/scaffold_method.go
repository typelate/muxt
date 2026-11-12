package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

// MethodSpec describes a receiver method a route calls and the receiver does
// not define.
type MethodSpec struct {
	// Receiver is the receiver type's name.
	Receiver string
	// Pointer declares the method on *Receiver, which a type does when its
	// other methods do, so the method sets stay one kind.
	Pointer bool
	// Name is the method name.
	Name string
	// Params are the method's parameters in order.
	Params []MethodParam
	// Results are the types of the method's results.
	Results []string
}

// MethodParam is one method parameter.
type MethodParam struct {
	// Name is the parameter name.
	Name string
	// Type is the parameter's type as Go source reads it, such as
	// "context.Context", qualified the way the receiver's package spells it.
	Type string
}

// stubLines is how many lines the printed declaration has: the signature, the
// body, and the closing brace.
const stubLines = 3

// MethodStub returns the formatted source of a method that has the shape of
// spec and returns nil for each result, for a person to fill in.
//
// The printer keeps a body on the signature's line unless the braces are on
// different lines, so each node sits on the line it has in the printed method.
// A type is read with the parser because the callers hold it as text, and only
// the type expression is parsed, never a declaration.
func MethodStub(spec MethodSpec) (string, error) {
	fset := token.NewFileSet()
	tf := fset.AddFile(spec.Name+".go", -1, (stubLines+1)*10)
	if !tf.SetLines([]int{0, 10, 20, 30}) {
		return "", fmt.Errorf("laying out method %s", spec.Name)
	}
	b := builder{file: tf}

	var receiver ast.Expr = b.ident(spec.Receiver, 1)
	if spec.Pointer {
		receiver = &ast.StarExpr{Star: b.pos(1), X: receiver}
	}
	params := &ast.FieldList{Opening: b.pos(1), Closing: b.pos(1)}
	for _, p := range spec.Params {
		tp, err := parser.ParseExpr(p.Type)
		if err != nil {
			return "", fmt.Errorf("type %q of parameter %s: %w", p.Type, p.Name, err)
		}
		params.List = append(params.List, &ast.Field{Names: []*ast.Ident{b.ident(p.Name, 1)}, Type: tp})
	}
	var results *ast.FieldList
	body := &ast.BlockStmt{Lbrace: b.pos(1), Rbrace: b.pos(stubLines)}
	if len(spec.Results) > 0 {
		results = &ast.FieldList{}
		ret := &ast.ReturnStmt{Return: b.pos(2)}
		for _, r := range spec.Results {
			tp, err := parser.ParseExpr(r)
			if err != nil {
				return "", fmt.Errorf("result type %q of method %s: %w", r, spec.Name, err)
			}
			results.List = append(results.List, &ast.Field{Type: tp})
			ret.Results = append(ret.Results, b.ident("nil", 2))
		}
		body.List = []ast.Stmt{ret}
	} else {
		body.Rbrace = b.pos(2)
	}
	decl := &ast.FuncDecl{
		Recv: &ast.FieldList{Opening: b.pos(1), List: []*ast.Field{{Type: receiver}}, Closing: b.pos(1)},
		Name: b.ident(spec.Name, 1),
		Type: &ast.FuncType{Func: b.pos(1), Params: params, Results: results},
		Body: body,
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, decl); err != nil {
		return "", fmt.Errorf("formatting method %s: %w", spec.Name, err)
	}
	buf.WriteByte('\n')
	return buf.String(), nil
}
