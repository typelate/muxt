package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"net/http"
	"strconv"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

// argumentParser renders the statements that bind a call's arguments from the
// request, and rewrites the call to use the locals they declare.
type argumentParser struct {
	file                   *File
	rdIdent                string
	config                 RoutesFileConfiguration
	validationFailureBlock ValidationErrorBlock
	parseErrBlock          func() *ast.BlockStmt
	// nestedCallErrBlock is what runs when a call passed as an argument
	// returns an error.
	nestedCallErrBlock func() *ast.BlockStmt
}

func appendParseArgumentStatements(statements []ast.Stmt, file *File, args []muxt.Argument, rdIdent string, config RoutesFileConfiguration, call *ast.CallExpr, validationFailureBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt) ([]ast.Stmt, error) {
	if parseErrBlock == nil {
		// Normal handlers accumulate scalar-parse failures into the template
		// data (and respond with the recorded error status). SSE handlers pass
		// their own factory to respond 400 before opening the event stream.
		parseErrBlock = func() *ast.BlockStmt { return templateDataParseErrBlock(file, rdIdent) }
	}
	p := argumentParser{
		file:                   file,
		rdIdent:                rdIdent,
		config:                 config,
		validationFailureBlock: validationFailureBlock,
		parseErrBlock:          parseErrBlock,
		nestedCallErrBlock:     func() *ast.BlockStmt { return templateDataNestedCallErrBlock(file, rdIdent) },
	}
	return p.appendCall(statements, args, call)
}

func (p argumentParser) appendCall(statements []ast.Stmt, args []muxt.Argument, call *ast.CallExpr) ([]ast.Stmt, error) {
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("expected function to be identifier")
	}
	if len(args) != len(call.Args) {
		return nil, fmt.Errorf("call %s was not resolved", fun.Name)
	}
	resultCount := 0
	for i, a := range call.Args {
		var err error
		switch arg := a.(type) {
		case *ast.CallExpr:
			if args[i].Type == muxt.ArgumentTypeRequestBodyJSON {
				statements, err = p.appendBodyJSON(statements, call, i, args[i])
				break
			}
			statements, err = p.appendNestedCall(statements, call, i, args[i], arg, resultCount)
			resultCount++
		case *ast.Ident:
			statements, err = p.appendIdentifier(statements, call, i, args[i], arg)
		default:
			err = fmt.Errorf("unsupported argument %s in call to %s", astgen.Format(a), fun.Name)
		}
		if err != nil {
			return nil, err
		}
	}
	return statements, nil
}

func (p argumentParser) appendBodyJSON(statements []ast.Stmt, call *ast.CallExpr, i int, argument muxt.Argument) ([]ast.Stmt, error) {
	decode, err := decodeJSONBodyStatements(p.file, bodyValueIdent, argument.ParamType(), p.parseErrBlock)
	if err != nil {
		return nil, err
	}
	call.Args[i] = ast.NewIdent(bodyValueIdent)
	return append(statements, decode...), nil
}

// appendNestedCall renders nested, an argument that is itself a call, as a
// call whose result the outer call takes as result<resultCount>.
func (p argumentParser) appendNestedCall(statements []ast.Stmt, call *ast.CallExpr, i int, nested muxt.Argument, nestedCall *ast.CallExpr, resultCount int) ([]ast.Stmt, error) {
	statements, err := p.appendCall(statements, nested.Arguments(), nestedCall)
	if err != nil {
		return nil, err
	}
	resultVarIdent := resultIdent + strconv.Itoa(resultCount)
	call.Args[i] = ast.NewIdent(resultVarIdent)

	funcIdent := nestedCall.Fun.(*ast.Ident).Name
	nestedCall.Fun = ast.NewIdent(funcIdent)
	if nested.IsMethod() {
		nestedCall.Fun = selector(receiverIdent, funcIdent)
	}

	receiverCall, err := callReceiverMethod(p.rdIdent, ast.NewIdent(resultVarIdent), nested.ResultShape(), funcIdent, nestedCall, p.nestedCallErrBlock())
	if err != nil {
		return nil, err
	}
	return append(statements, receiverCall.DefineStmts()...), nil
}

func (p argumentParser) appendIdentifier(statements []ast.Stmt, call *ast.CallExpr, i int, argument muxt.Argument, arg *ast.Ident) ([]ast.Stmt, error) {
	if argument.Type == muxt.ArgumentTypeExecute || argument.Type == muxt.ArgumentTypeSignalsCallback {
		// Render and signals callbacks are validated and wired into the
		// call in the sse handler assembly. They are not parsed from
		// the request.
		return statements, nil
	}
	ident := arg.Name
	if argument.Type == muxt.ArgumentTypeRequestPathValue {
		ident = pathParamIdent(arg.Name)
		call.Args[i] = ast.NewIdent(ident)
	}
	if argument.Direct() {
		if !argument.Declares() {
			return statements, nil
		}
		return p.appendDirect(statements, argument, arg, ident)
	}
	switch argument.Type {
	case muxt.ArgumentTypeRequestPathValue, muxt.ArgumentTypeLastEventID, muxt.ArgumentTypeRequestForm, muxt.ArgumentTypeRequestMultipartForm:
		if !argument.Declares() {
			return statements, nil
		}
		return p.appendParsed(statements, argument, arg, ident)
	default:
		return nil, mismatchedArgumentError(p.file, argument)
	}
}

// appendDirect declares the local for an argument whose request value is
// assignable to its parameter as it is.
func (p argumentParser) appendDirect(statements []ast.Stmt, argument muxt.Argument, arg *ast.Ident, ident string) ([]ast.Stmt, error) {
	switch argument.Type {
	case muxt.ArgumentTypeRequestForm:
		declare, err := typedVar(p.file, arg.Name, argument.ParamType(), requestField("Form"))
		if err != nil {
			return nil, err
		}
		return append(statements, callParseForm(p.file), declare), nil
	case muxt.ArgumentTypeRequestMultipartForm:
		declare, err := typedVar(p.file, arg.Name, argument.ParamType(), requestField("MultipartForm"))
		if err != nil {
			return nil, err
		}
		return append(statements, callParseMultipartForm(p.file, p.config, p.parseErrBlock()), declare), nil
	case muxt.ArgumentTypeRequestContext:
		return append(statements, contextAssignment(muxt.TemplateNameScopeIdentifierContext)), nil
	case muxt.ArgumentTypeRequestBody, muxt.ArgumentTypeRequestPathValue, muxt.ArgumentTypeLastEventID:
		src, err := requestArgumentSource(argument)
		if err != nil {
			return nil, err
		}
		return append(statements, singleAssignment(token.DEFINE, ast.NewIdent(ident))(src)), nil
	}
	return statements, nil
}

// appendParsed declares the local for an argument parsed from a string or
// bound into a struct.
func (p argumentParser) appendParsed(statements []ast.Stmt, argument muxt.Argument, arg *ast.Ident, ident string) ([]ast.Stmt, error) {
	switch argument.Type {
	case muxt.ArgumentTypeRequestForm:
		return appendParseFormToStructStatements(statements, p.file, arg, argument, p.validationFailureBlock, p.parseErrBlock)
	case muxt.ArgumentTypeRequestMultipartForm:
		return appendParseMultipartFormToStructStatements(statements, p.file, arg, argument, p.validationFailureBlock, p.parseErrBlock, p.config)
	}
	src, err := requestArgumentSource(argument)
	if err != nil {
		return nil, err
	}
	parsed, err := scalarParse{
		tmp: arg.Name + "Parsed", str: src, typ: argument.ParamType(), method: argument.UnmarshalMethod(),
		assign: singleAssignment(token.DEFINE, ast.NewIdent(ident)), errBlock: p.parseErrBlock(),
	}.statements(p.file)
	if err != nil {
		return nil, err
	}
	return append(statements, parsed...), nil
}

func mismatchedArgumentError(file *File, argument muxt.Argument) error {
	if argument.ScopeType().IsZero() {
		return fmt.Errorf("failed to determine type for %s", argument.Identifier)
	}
	paramType, err := file.TypeExpr(argument.ParamType())
	if err != nil {
		return err
	}
	scopeType, err := file.TypeExpr(argument.ScopeType())
	if err != nil {
		return err
	}
	return fmt.Errorf("method expects type %s but %s is %s", astgen.Format(paramType), argument.Identifier, astgen.Format(scopeType))
}

// templateDataParseErrBlock builds the standard scalar-parse failure block used
// by normal handlers: it appends the error to the template data and sets the
// error status code to 400.
func templateDataParseErrBlock(file *File, rdIdent string) *ast.BlockStmt {
	b := appendTemplateDataError(file, rdIdent, ast.NewIdent(errIdent))
	b.List = append(b.List, assignTemplateDataErrStatusCode(file, rdIdent, http.StatusBadRequest))
	return b
}

// templateDataNestedCallErrBlock is the block normal handlers run when a
// call passed as an argument fails: it appends the error to the template
// data and sets the error status code to 500.
func templateDataNestedCallErrBlock(file *File, rdIdent string) *ast.BlockStmt {
	b := appendTemplateDataError(file, rdIdent, ast.NewIdent(errIdent))
	b.List = append(b.List, assignTemplateDataErrStatusCode(file, rdIdent, http.StatusInternalServerError))
	return b
}

type receiverMethodCall struct {
	VarDecl ast.Stmt        // var err/ok declaration; nil for 1-result
	Assign  *ast.AssignStmt // dataVar[, err/ok] = call
	Check   ast.Stmt        // if err != nil / if !ok; nil for 1-result
	SetOkay *ast.AssignStmt // td.okay = true; nil for error-returning methods
}

// DefineStmts returns statements for use as an inline call where the result
// variable is being defined. It sets Assign to use := and includes the Check
// if present.
func (r *receiverMethodCall) DefineStmts() []ast.Stmt {
	r.Assign.Tok = token.DEFINE
	stmts := []ast.Stmt{r.Assign}
	if r.Check != nil {
		stmts = append(stmts, r.Check)
	}
	return stmts
}

func (r *receiverMethodCall) Stmts() []ast.Stmt {
	var stmts []ast.Stmt
	if r.VarDecl != nil {
		stmts = append(stmts, r.VarDecl)
	}
	stmts = append(stmts, r.Assign)
	if r.Check != nil {
		stmts = append(stmts, r.Check)
	}
	if r.SetOkay != nil {
		stmts = append(stmts, r.SetOkay)
	}
	return stmts
}

func callReceiverMethod(rdIdent string, dataVar ast.Expr, shape muxt.ResultShape, callIdent string, call *ast.CallExpr, errBody *ast.BlockStmt) (*receiverMethodCall, error) {
	switch shape {
	default:
		return nil, fmt.Errorf("method %s has no results it should have one or two", callIdent)
	case muxt.ResultShapeData:
		return &receiverMethodCall{
			Assign: &ast.AssignStmt{Lhs: []ast.Expr{dataVar}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
			SetOkay: &ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{
				X:   ast.NewIdent(rdIdent),
				Sel: ast.NewIdent(TemplateDataFieldIdentifierOkay),
			}}, Tok: token.ASSIGN, Rhs: []ast.Expr{astgen.Bool(true)}},
		}, nil
	case muxt.ResultShapeDataError:
		return &receiverMethodCall{
			VarDecl: varDecl(errIdent, ast.NewIdent("error"), nil),
			Assign:  &ast.AssignStmt{Lhs: []ast.Expr{dataVar, ast.NewIdent(errIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
			Check: &ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: errBody,
			},
		}, nil
	case muxt.ResultShapeDataOK:
		return &receiverMethodCall{
			VarDecl: varDecl(okIdent, ast.NewIdent("bool"), nil),
			Assign:  &ast.AssignStmt{Lhs: []ast.Expr{dataVar, ast.NewIdent(okIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
			Check: &ast.IfStmt{
				Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent(okIdent)},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{}}},
			},
			SetOkay: &ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{
				X:   ast.NewIdent(rdIdent),
				Sel: ast.NewIdent(TemplateDataFieldIdentifierOkay),
			}}, Tok: token.ASSIGN, Rhs: []ast.Expr{astgen.Bool(true)}},
		}, nil
	}
}

// decodeJSONBodyStatements declares valueIdent with the parameter type and
// decodes the JSON request body into it:
//
//	var bodyValue T
//	if err := json.NewDecoder(request.Body).Decode(&bodyValue); err != nil { <parseErrBlock> }
func decodeJSONBodyStatements(file *File, valueIdent string, paramType source.Type, parseErrBlock func() *ast.BlockStmt) ([]ast.Stmt, error) {
	declare, err := typedVar(file, valueIdent, paramType, nil)
	if err != nil {
		return nil, err
	}
	decode := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   astgen.Call(file, "json", "encoding/json", "NewDecoder", requestField("Body")),
			Sel: ast.NewIdent("Decode"),
		},
		Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(valueIdent)}},
	}
	checkErr := &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{decode}},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: parseErrBlock(),
	}
	return []ast.Stmt{declare, checkErr}, nil
}

// lastEventIDHeader is the canonical request header the lastEventID argument is
// sourced from. http.Header.Get canonicalizes lookups, so this matches a
// client's "Last-Event-ID" as well.
const lastEventIDHeader = "Last-Event-Id"

// requestArgumentSource returns the expression a scalar argument is parsed
// from: request.Body for body, request.Header.Get("Last-Event-Id") for
// lastEventID, request.PathValue(argument.Identifier) for a path value, and
// an error for any other argument type.
func requestArgumentSource(argument muxt.Argument) (ast.Expr, error) {
	switch argument.Type {
	case muxt.ArgumentTypeRequestBody:
		return requestField("Body"), nil
	case muxt.ArgumentTypeLastEventID:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: requestField("Header"), Sel: ast.NewIdent("Get")},
			Args: []ast.Expr{astgen.String(lastEventIDHeader)},
		}, nil
	case muxt.ArgumentTypeRequestPathValue:
		return &ast.CallExpr{
			Fun:  requestField(requestPathValue),
			Args: []ast.Expr{astgen.String(argument.Identifier)},
		}, nil
	default:
		return nil, fmt.Errorf("no request source for argument %s", argument.Identifier)
	}
}

func contextAssignment(ident string) *ast.AssignStmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(ident)},
		Rhs: []ast.Expr{&ast.CallExpr{Fun: requestField(httpRequestContextMethod)}},
	}
}

func singleAssignment(assignTok token.Token, result ast.Expr) func(exp ast.Expr) ast.Stmt {
	return func(exp ast.Expr) ast.Stmt {
		return &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{exp},
		}
	}
}
