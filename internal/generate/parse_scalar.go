package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

// scalarParse describes parsing str into a value of typ. On a parse failure it
// runs errBlock, which callers supply so the failure can be handled differently
// per context (normal handlers accumulate into the template data; SSE handlers
// respond 400 before establishing the stream).
type scalarParse struct {
	tmp         string
	str         ast.Expr
	typ         source.Type
	method      muxt.UnmarshalMethod
	validations []ast.Stmt
	assign      func(ast.Expr) ast.Stmt
	errBlock    *ast.BlockStmt
}

// strconvParser calls the strconv function for a method. convert is set when
// that function returns a wider type than the target.
type strconvParser struct {
	call    func(astgen.ImportManager, ast.Expr) *ast.CallExpr
	convert bool
}

func parseFloat(size int) func(astgen.ImportManager, ast.Expr) *ast.CallExpr {
	return func(im astgen.ImportManager, str ast.Expr) *ast.CallExpr {
		return astgen.StrconvParseFloatCall(im, str, size)
	}
}

var strconvParsers = map[muxt.UnmarshalMethod]strconvParser{
	muxt.UnmarshalBool:    {call: astgen.StrconvParseBoolCall},
	muxt.UnmarshalInt:     {call: astgen.StrconvAtoiCall},
	muxt.UnmarshalInt8:    {call: astgen.StrconvParseInt8Call, convert: true},
	muxt.UnmarshalInt16:   {call: astgen.StrconvParseInt16Call, convert: true},
	muxt.UnmarshalInt32:   {call: astgen.StrconvParseInt32Call, convert: true},
	muxt.UnmarshalInt64:   {call: astgen.StrconvParseInt64Call},
	muxt.UnmarshalUint:    {call: astgen.StrconvParseUint0Call, convert: true},
	muxt.UnmarshalUint8:   {call: astgen.StrconvParseUint8Call, convert: true},
	muxt.UnmarshalUint16:  {call: astgen.StrconvParseUint16Call, convert: true},
	muxt.UnmarshalUint32:  {call: astgen.StrconvParseUint32Call, convert: true},
	muxt.UnmarshalUint64:  {call: astgen.StrconvParseUint64Call},
	muxt.UnmarshalFloat32: {call: parseFloat(32), convert: true},
	muxt.UnmarshalFloat64: {call: parseFloat(64)},
}

func (p scalarParse) statements(file *File) ([]ast.Stmt, error) {
	typeExpr, err := file.TypeExpr(p.typ)
	if err != nil {
		return nil, err
	}
	switch p.method {
	case muxt.UnmarshalString:
		return p.stringStatements(), nil
	case muxt.UnmarshalTextUnmarshaler:
		return p.textUnmarshalerStatements(typeExpr), nil
	}
	parser, ok := strconvParsers[p.method]
	if !ok {
		return nil, fmt.Errorf("unsupported type: %s", astgen.Format(typeExpr))
	}
	assign := p.assign
	if parser.convert {
		assign = func(exp ast.Expr) ast.Stmt { return p.assign(astgen.Convert(typeExpr, exp)) }
	}
	return parseBlock(p.tmp, parser.call(file, p.str), p.validations, p.errBlock, assign), nil
}

func (p scalarParse) stringStatements() []ast.Stmt {
	if len(p.validations) == 0 {
		return []ast.Stmt{p.assign(p.str)}
	}
	define := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(p.tmp)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{p.str},
	}
	return slices.Concat([]ast.Stmt{define}, p.validations, []ast.Stmt{p.assign(ast.NewIdent(p.tmp))})
}

func (p scalarParse) textUnmarshalerStatements(typeExpr ast.Expr) []ast.Stmt {
	unmarshal := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(p.tmp), Sel: ast.NewIdent("UnmarshalText")},
		Args: []ast.Expr{astgen.Convert(&ast.ArrayType{Elt: ast.NewIdent("byte")}, p.str)},
	}
	return []ast.Stmt{
		varDecl(p.tmp, typeExpr, nil),
		&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{unmarshal}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: p.errBlock,
		},
		p.assign(ast.NewIdent(p.tmp)),
	}
}

func parseBlock(tmpIdent string, parseCall ast.Expr, validations []ast.Stmt, errBlock *ast.BlockStmt, handleResult func(out ast.Expr) ast.Stmt) []ast.Stmt {
	callParse := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(tmpIdent), ast.NewIdent(errIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{parseCall},
	}
	errCheck := &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: errBlock,
	}
	if len(validations) > 0 {
		errCheck.Else = &ast.BlockStmt{List: validations}
	}
	return []ast.Stmt{callParse, errCheck, handleResult(ast.NewIdent(tmpIdent))}
}
