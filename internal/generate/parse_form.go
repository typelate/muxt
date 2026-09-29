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

func appendParseFormToStructStatements(statements []ast.Stmt, file *File, arg *ast.Ident, argument muxt.Argument, validationBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt) ([]ast.Stmt, error) {
	return appendStructFieldParseStatements(statements, file, arg, argument, validationBlock, parseErrBlock, callParseForm(file))
}

// appendStructFieldParseStatements renders the per-field parse statements for
// a form or multipart struct parameter from the field bindings resolved by
// muxt.ResolveCall. Used by both `form` (parseCall = callParseForm(file)) and
// `multipart` (parseCall = callParseMultipartForm(...)).
func appendStructFieldParseStatements(statements []ast.Stmt, file *File, arg *ast.Ident, argument muxt.Argument, validationBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt, parseCall ast.Stmt) ([]ast.Stmt, error) {
	const parsedVariableName = "value"
	statements = append(statements, parseCall)

	declareVar, err := formVariableDeclaration(file, arg, argument.ParamType())
	if err != nil {
		return nil, err
	}
	statements = append(statements, declareVar)

	for _, fb := range argument.FormFields() {
		if fb.FileHeader {
			if fb.Slice {
				statements = append(statements, fileHeaderSliceAssignment(arg, fb.Name, fb.InputName))
			} else {
				statements = append(statements, fileHeaderSingleAssignment(arg, fb.Name, fb.InputName))
			}
			continue
		}

		validations := renderValidations(file, ast.NewIdent(parsedVariableName), fb.Validations, validationBlock)
		if fb.Slice {
			parseResult := func(expr ast.Expr) ast.Stmt {
				return &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fb.Name)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{astgen.CallBuiltinAppend(&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fb.Name)}, expr)},
				}
			}
			parseStatements, err := generateParseValueFromStringStatements(file, parsedVariableName, ast.NewIdent("val"), fb.Elem(), fb.Method, validations, parseResult, parseErrBlock())
			if err != nil {
				return nil, fmt.Errorf("failed to generate parse statements for %s field %s: %w", arg.Name, fb.Name, err)
			}
			statements = append(statements, &ast.RangeStmt{
				Key:   ast.NewIdent("_"),
				Value: ast.NewIdent("val"),
				Tok:   token.DEFINE,
				X:     &ast.IndexExpr{X: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Form")}, Index: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(fb.InputName)}},
				Body:  &ast.BlockStmt{List: parseStatements},
			})
		} else {
			parseResult := func(expr ast.Expr) ast.Stmt {
				return &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fb.Name)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{expr},
				}
			}
			str := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("FormValue")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(fb.InputName)}}}
			parseStatements, err := generateParseValueFromStringStatements(file, parsedVariableName, str, fb.Elem(), fb.Method, validations, parseResult, parseErrBlock())
			if err != nil {
				return nil, fmt.Errorf("failed to generate parse statements for %s field %s: %w", arg.Name, fb.Name, err)
			}
			if len(parseStatements) > 1 {
				statements = append(statements, &ast.BlockStmt{
					List: parseStatements,
				})
			} else {
				statements = append(statements, parseStatements...)
			}
		}
	}

	return statements, nil
}

// appendParseMultipartFormToStructStatements is a thin wrapper over
// appendStructFieldParseStatements that emits a ParseMultipartForm call.
// FileHeader field bindings (from request.MultipartForm.File) are resolved by
// muxt.ResolveCall; all other field-binding behavior is shared with the form
// codepath.
func appendParseMultipartFormToStructStatements(statements []ast.Stmt, file *File, arg *ast.Ident, argument muxt.Argument, validationBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt, config RoutesFileConfiguration) ([]ast.Stmt, error) {
	return appendStructFieldParseStatements(statements, file, arg, argument, validationBlock, parseErrBlock, callParseMultipartForm(file, config, parseErrBlock()))
}

// fileHeaderSingleAssignment emits:
//
//	if request.MultipartForm != nil {
//	    if fhs := request.MultipartForm.File["<inputName>"]; len(fhs) > 0 {
//	        <arg>.Field = fhs[0]
//	    }
//	}
func fileHeaderSingleAssignment(arg *ast.Ident, fieldName, inputName string) ast.Stmt {
	const tmp = "fhs"
	inner := &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("MultipartForm")},
					Sel: ast.NewIdent("File"),
				},
				Index: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inputName)},
			}},
		},
		Cond: &ast.BinaryExpr{
			X:  astgen.CallBuiltinLen(ast.NewIdent(tmp)),
			Op: token.GTR,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fieldName)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(tmp), Index: &ast.BasicLit{Kind: token.INT, Value: "0"}}},
			},
		}},
	}
	return wrapInMultipartFormNotNil(inner)
}

// fileHeaderSliceAssignment emits:
//
//	if request.MultipartForm != nil {
//	    <arg>.Field = request.MultipartForm.File["<inputName>"]
//	}
func fileHeaderSliceAssignment(arg *ast.Ident, fieldName, inputName string) ast.Stmt {
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fieldName)}},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("MultipartForm")},
				Sel: ast.NewIdent("File"),
			},
			Index: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inputName)},
		}},
	}
	return wrapInMultipartFormNotNil(assign)
}

// wrapInMultipartFormNotNil wraps stmt in `if request.MultipartForm != nil { stmt }`.
// ParseMultipartForm leaves request.MultipartForm nil if it returns an error,
// so we guard accesses to avoid nil-pointer panics on malformed bodies.
func wrapInMultipartFormNotNil(stmt ast.Stmt) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("MultipartForm")},
			Op: token.NEQ,
			Y:  ast.NewIdent("nil"),
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{stmt}},
	}
}

func formVariableDeclaration(file *File, arg *ast.Ident, tp source.Type) (*ast.DeclStmt, error) {
	typeExp, err := file.TypeExpr(tp)
	if err != nil {
		return nil, err
	}
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(arg.Name)},
					Type:  typeExp,
				},
			},
		},
	}, nil
}

func formVariableAssignment(file *File, arg *ast.Ident, tp source.Type) (*ast.DeclStmt, error) {
	typeExp, err := file.TypeExpr(tp)
	if err != nil {
		return nil, err
	}
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(arg.Name)},
					Type:  typeExp,
					Values: []ast.Expr{
						&ast.SelectorExpr{
							X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
							Sel: ast.NewIdent("Form"),
						},
					},
				},
			},
		},
	}, nil
}

// callParseForm emits:
//
//	if err := request.ParseForm(); err != nil {
//	    http.Error(response, err.Error(), http.StatusBadRequest)
//	    return
//	}
func callParseForm(file *File) *ast.IfStmt {
	errBlock := &ast.BlockStmt{List: []ast.Stmt{
		&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), astgen.CallError(errIdent), http.StatusBadRequest)},
		&ast.ReturnStmt{},
	}}
	return &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(errIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent("ParseForm"),
				},
			}},
		},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: errBlock,
	}
}

// callParseMultipartForm emits:
//
//	if err := request.ParseMultipartForm(<maxMemory>); err != nil && !errors.Is(err, http.ErrNotMultipart) {
//	    <errBlock>
//	}
//
// The caller supplies errBlock so the failure is handled per handler
// kind: normal handlers accumulate the error into the template data
// with a 400 status; SSE handlers respond 400 and return before the
// event stream is established.
//
// http.ErrNotMultipart is exempted because ParseMultipartForm calls ParseForm
// internally, so url-encoded POSTs to a multipart route still populate
// request.PostForm — the receiver method should run with text fields bound
// (file fields stay nil). Other errors (truncated body, bad boundary,
// underlying ParseForm failures) are real.
func callParseMultipartForm(file *File, config RoutesFileConfiguration, errBlock *ast.BlockStmt) *ast.IfStmt {
	maxMemory := config.MultipartMaxMemory
	if maxMemory <= 0 {
		maxMemory = DefaultMultipartMaxMemory
	}
	httpPkg := astgen.AddNetHTTP(file)
	notNil := &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: ast.NewIdent("nil")}
	notErrNotMultipart := &ast.UnaryExpr{
		Op: token.NOT,
		X: astgen.Call(file, "", "errors", "Is",
			ast.NewIdent(errIdent),
			&ast.SelectorExpr{X: ast.NewIdent(httpPkg), Sel: ast.NewIdent("ErrNotMultipart")},
		),
	}
	return &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(errIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent("ParseMultipartForm"),
				},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(maxMemory, 10)}},
			}},
		},
		Cond: &ast.BinaryExpr{X: notNil, Op: token.LAND, Y: notErrNotMultipart},
		Body: errBlock,
	}
}

// multipartVariableAssignment emits `var <arg> <Type> = request.MultipartForm`
// for raw-mode multipart binding.
func multipartVariableAssignment(file *File, arg *ast.Ident, tp source.Type) (*ast.DeclStmt, error) {
	typeExp, err := file.TypeExpr(tp)
	if err != nil {
		return nil, err
	}
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(arg.Name)},
					Type:  typeExp,
					Values: []ast.Expr{
						&ast.SelectorExpr{
							X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
							Sel: ast.NewIdent("MultipartForm"),
						},
					},
				},
			},
		},
	}, nil
}
