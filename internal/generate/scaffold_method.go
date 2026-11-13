package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"strings"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
	"github.com/typelate/muxt/template/html"
)

// MethodStubs returns a stub for each method the route name calls that
// receiver does not define, as muxt infers its signature. A method the receiver
// has gives no stub. Stubs use a pointer receiver when the type's own methods
// do. Types are spelled the way receiver's package spells them.
func MethodStubs(pkg source.Package, receiver *types.Named, name string, checker muxt.Checker) (string, error) {
	templates, err := html.New("").Parse(DefineBlock(name, FragmentBody))
	if err != nil {
		return "", fmt.Errorf("parsing %q: %w", name, err)
	}
	pointer := hasPointerMethod(receiver)
	probe := source.Package{Fset: pkg.Fset, Types: pkg.Types, Variables: []source.Variable{{Name: "probe", Set: templates}}}
	defs, err := muxt.ResolveDefinitions(probe, receiver, checker)
	if err != nil {
		return "", err
	}
	var stubs []string
	for _, def := range defs {
		for _, inferred := range def.SynthesizedMethods() {
			method, _, _ := strings.Cut(inferred, "(")
			spec, err := methodSpec(receiver, pointer, method)
			if err != nil {
				return "", err
			}
			stub, err := MethodStub(spec)
			if err != nil {
				return "", err
			}
			stubs = append(stubs, stub)
		}
	}
	return strings.Join(stubs, "\n"), nil
}

// hasPointerMethod reports whether receiver declares a method on its pointer
// type. Only declared methods count: a method has a position only when it is
// written in source, and muxt adds the ones it infers without one.
func hasPointerMethod(receiver *types.Named) bool {
	for method := range receiver.Methods() {
		if !method.Pos().IsValid() {
			continue
		}
		if _, ok := method.Signature().Recv().Type().(*types.Pointer); ok {
			return true
		}
	}
	return false
}

// methodSpec describes the method named name that muxt added to receiver.
func methodSpec(receiver *types.Named, pointer bool, name string) (MethodSpec, error) {
	for method := range receiver.Methods() {
		if method.Name() != name {
			continue
		}
		sig := method.Signature()
		spec := MethodSpec{
			Receiver: receiver.Obj().Name(),
			Pointer:  pointer,
			Name:     name,
			Qualify: func(p *types.Package) string {
				if p == receiver.Obj().Pkg() {
					return ""
				}
				return p.Name()
			},
		}
		for param := range sig.Params().Variables() {
			spec.Params = append(spec.Params, MethodParam{Name: param.Name(), Type: param.Type()})
		}
		for result := range sig.Results().Variables() {
			spec.Results = append(spec.Results, result.Type())
		}
		return spec, nil
	}
	return MethodSpec{}, fmt.Errorf("method %s inferred for %s is not on the type", name, receiver.Obj().Name())
}

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
	// Results are the method's result types.
	Results []types.Type
	// Qualify spells a type's package, as the receiver's package would. Nil
	// qualifies every package by its path.
	Qualify types.Qualifier
}

// MethodParam is one method parameter.
type MethodParam struct {
	// Name is the parameter name.
	Name string
	// Type is the parameter's type.
	Type types.Type
}

// stubLines is how many lines the printed declaration has: the signature, the
// body, and the closing brace.
const stubLines = 3

// MethodStub returns the formatted source of a method that has the shape of
// spec and returns nil for each result, for a person to fill in.
//
// The printer keeps a body on the signature's line unless the braces are on
// different lines, so each node sits on the line it has in the printed method.
// Each type is spelled once, here, by types.TypeString and read back as a type
// expression; only the expression is parsed, never a declaration.
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
		tp, err := typeExpr(p.Type, spec.Qualify)
		if err != nil {
			return "", fmt.Errorf("type of parameter %s: %w", p.Name, err)
		}
		params.List = append(params.List, &ast.Field{Names: []*ast.Ident{b.ident(p.Name, 1)}, Type: tp})
	}
	var results *ast.FieldList
	body := &ast.BlockStmt{Lbrace: b.pos(1), Rbrace: b.pos(stubLines)}
	if len(spec.Results) > 0 {
		results = &ast.FieldList{}
		ret := &ast.ReturnStmt{Return: b.pos(2)}
		for _, r := range spec.Results {
			tp, err := typeExpr(r, spec.Qualify)
			if err != nil {
				return "", fmt.Errorf("result type of method %s: %w", spec.Name, err)
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

// typeExpr returns the expression that spells tp as qualify would.
func typeExpr(tp types.Type, qualify types.Qualifier) (ast.Expr, error) {
	text := types.TypeString(tp, qualify)
	expr, err := parser.ParseExpr(text)
	if err != nil {
		return nil, fmt.Errorf("reading type %q: %w", text, err)
	}
	return expr, nil
}
