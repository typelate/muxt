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

func appendParseArgumentStatements(statements []ast.Stmt, file *File, args []muxt.Argument, rdIdent string, config RoutesFileConfiguration, call *ast.CallExpr, validationFailureBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt) ([]ast.Stmt, error) {
	if parseErrBlock == nil {
		// Normal handlers accumulate scalar-parse failures into the template
		// data (and respond with the recorded error status). SSE handlers pass
		// their own factory to respond 400 before opening the event stream.
		parseErrBlock = func() *ast.BlockStmt { return templateDataParseErrBlock(file, rdIdent) }
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("expected function to be identifier")
	}
	if len(args) != len(call.Args) {
		return nil, fmt.Errorf("call %s was not resolved", fun.Name)
	}
	resultCount := 0
	for i, a := range call.Args {
		switch arg := a.(type) {
		default:
		case *ast.CallExpr:
			nestedArg := args[i]
			if nestedArg.Type == muxt.ArgumentTypeRequestBodyJSON {
				const bodyValueIdent = "bodyValue"
				decodeStatements, err := decodeJSONBodyStatements(file, bodyValueIdent, nestedArg.ParamType(), parseErrBlock)
				if err != nil {
					return nil, err
				}
				statements = append(statements, decodeStatements...)
				call.Args[i] = ast.NewIdent(bodyValueIdent)
				continue
			}
			parseArgStatements, err := appendParseArgumentStatements(statements, file, nestedArg.Arguments(), rdIdent, config, arg, validationFailureBlock, parseErrBlock)
			if err != nil {
				return nil, err
			}
			resultVarIdent := "result" + strconv.Itoa(resultCount)
			call.Args[i] = ast.NewIdent(resultVarIdent)
			resultCount++

			funcIdent := arg.Fun.(*ast.Ident).Name

			if nestedArg.IsMethod() {
				arg.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(receiverIdent),
					Sel: ast.NewIdent(funcIdent),
				}
			} else {
				arg.Fun = ast.NewIdent(funcIdent)
			}

			errBody := appendTemplateDataError(file, rdIdent, ast.NewIdent(errIdent))
			errBody.List = append(errBody.List, assignTemplateDataErrStatusCode(file, rdIdent, http.StatusInternalServerError))
			nestedCall, err := callReceiverMethod(rdIdent, ast.NewIdent(resultVarIdent), nestedArg.ResultShape(), funcIdent, arg, errBody)
			if err != nil {
				return nil, err
			}

			statements = append(parseArgStatements, nestedCall.DefineStmts()...)
		case *ast.Ident:
			argument := args[i]
			if argument.Type == muxt.ArgumentTypeExecute || argument.Type == muxt.ArgumentTypeSignalsCallback {
				// Render and signals callbacks are validated and wired into the
				// call in the sse handler assembly. They are not parsed from
				// the request.
				continue
			}
			name := arg.Name
			ident := name
			if argument.Type == muxt.ArgumentTypeRequestPathValue {
				ident = pathParamIdent(name)
				call.Args[i] = ast.NewIdent(ident)
			}
			if argument.Direct() {
				if argument.Declares() {
					switch argument.Type {
					case muxt.ArgumentTypeRequestForm:
						declareFormVar, err := typedVar(file, arg.Name, argument.ParamType(), requestField("Form"))
						if err != nil {
							return nil, err
						}
						statements = append(statements, callParseForm(file), declareFormVar)
					case muxt.ArgumentTypeRequestMultipartForm:
						declareMultipartVar, err := typedVar(file, arg.Name, argument.ParamType(), requestField("MultipartForm"))
						if err != nil {
							return nil, err
						}
						statements = append(statements, callParseMultipartForm(file, config, parseErrBlock()), declareMultipartVar)
					case muxt.ArgumentTypeRequestContext:
						statements = append(statements, contextAssignment(muxt.TemplateNameScopeIdentifierContext))
					case muxt.ArgumentTypeRequestBody, muxt.ArgumentTypeRequestPathValue, muxt.ArgumentTypeLastEventID:
						src, err := requestArgumentSource(argument)
						if err != nil {
							return nil, err
						}
						statements = append(statements, singleAssignment(token.DEFINE, ast.NewIdent(ident))(src))
					}
				}
				continue
			}
			switch argument.Type {
			case muxt.ArgumentTypeRequestPathValue, muxt.ArgumentTypeLastEventID:
				if !argument.Declares() {
					continue
				}
				src, err := requestArgumentSource(argument)
				if err != nil {
					return nil, err
				}
				s, err := scalarParse{
					tmp: name + "Parsed", str: src, typ: argument.ParamType(), method: argument.UnmarshalMethod(),
					assign: singleAssignment(token.DEFINE, ast.NewIdent(ident)), errBlock: parseErrBlock(),
				}.statements(file)
				if err != nil {
					return nil, err
				}
				statements = append(statements, s...)
			case muxt.ArgumentTypeRequestForm:
				if !argument.Declares() {
					continue
				}
				s, err := appendParseFormToStructStatements(statements, file, arg, argument, validationFailureBlock, parseErrBlock)
				if err != nil {
					return nil, err
				}
				statements = s
			case muxt.ArgumentTypeRequestMultipartForm:
				if !argument.Declares() {
					continue
				}
				s, err := appendParseMultipartFormToStructStatements(statements, file, arg, argument, validationFailureBlock, parseErrBlock, config)
				if err != nil {
					return nil, err
				}
				statements = s
			default:
				if argument.ScopeType().IsZero() {
					return nil, fmt.Errorf("failed to determine type for %s", name)
				}
				pt, _ := file.TypeExpr(argument.ParamType())
				at, _ := file.TypeExpr(argument.ScopeType())
				return nil, fmt.Errorf("method expects type %s but %s is %s", astgen.Format(pt), arg.Name, astgen.Format(at))
			}
		}
	}
	return statements, nil
}

// templateDataParseErrBlock builds the standard scalar-parse failure block used
// by normal handlers: it appends the error to the template data and sets the
// error status code to 400.
func templateDataParseErrBlock(file *File, rdIdent string) *ast.BlockStmt {
	b := appendTemplateDataError(file, rdIdent, ast.NewIdent(errIdent))
	b.List = append(b.List, assignTemplateDataErrStatusCode(file, rdIdent, http.StatusBadRequest))
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
	const okIdent = "ok"
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
			VarDecl: &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errIdent)}, Type: ast.NewIdent("error")}}}},
			Assign:  &ast.AssignStmt{Lhs: []ast.Expr{dataVar, ast.NewIdent(errIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
			Check: &ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: errBody,
			},
		}, nil
	case muxt.ResultShapeDataOK:
		return &receiverMethodCall{
			VarDecl: &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(okIdent)}, Type: ast.NewIdent("bool")}}}},
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
	typeExpr, err := file.TypeExpr(paramType)
	if err != nil {
		return nil, err
	}
	declare := &ast.DeclStmt{Decl: &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueIdent)}, Type: typeExpr}},
	}}
	decode := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: astgen.Call(file, "json", "encoding/json", "NewDecoder",
				&ast.SelectorExpr{
					X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent("Body"),
				},
			),
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
		return &ast.SelectorExpr{
			X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
			Sel: ast.NewIdent("Body"),
		}, nil
	case muxt.ArgumentTypeLastEventID:
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Header")},
				Sel: ast.NewIdent("Get"),
			},
			Args: []ast.Expr{astgen.String(lastEventIDHeader)},
		}, nil
	case muxt.ArgumentTypeRequestPathValue:
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
				Sel: ast.NewIdent(requestPathValue),
			},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(argument.Identifier)}},
		}, nil
	default:
		return nil, fmt.Errorf("no request source for argument %s", argument.Identifier)
	}
}

func contextAssignment(ident string) *ast.AssignStmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(ident)},
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
				Sel: ast.NewIdent(httpRequestContextMethod),
			},
		}},
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
