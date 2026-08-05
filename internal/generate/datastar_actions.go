package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	datastarActionsTypeIdent = "DatastarActions"
	datastarActionTypeIdent  = "DatastarAction"

	datastarActionReceiverName  = "actions"
	datastarActionValueReceiver = "action"

	datastarActionFieldMethod = "method"
	datastarActionFieldURL    = "url"
)

// datastarActionOptions is the fluent option vocabulary, in both declaration
// and render order. Each setter copies the action (value receiver, value
// return), so an action can be reused with different option chains.
var datastarActionOptions = []struct {
	Method  string
	Field   string
	Key     string
	GoType  string
	IsJSStr bool
}{
	{Method: "ContentType", Field: "contentType", Key: "contentType", GoType: "string", IsJSStr: true},
	{Method: "OpenWhenHidden", Field: "openWhenHidden", Key: "openWhenHidden", GoType: "bool"},
	{Method: "Selector", Field: "selector", Key: "selector", GoType: "string", IsJSStr: true},
	{Method: "Retry", Field: "retry", Key: "retry", GoType: "string", IsJSStr: true},
	{Method: "RequestCancellation", Field: "requestCancellation", Key: "requestCancellation", GoType: "string", IsJSStr: true},
	{Method: "RetryInterval", Field: "retryInterval", Key: "retryInterval", GoType: "int"},
	{Method: "RetryScaler", Field: "retryScaler", Key: "retryScaler", GoType: "float64"},
	{Method: "RetryMaxWait", Field: "retryMaxWait", Key: "retryMaxWait", GoType: "int"},
	{Method: "RetryMaxCount", Field: "retryMaxCount", Key: "retryMaxCount", GoType: "int"},
}

// datastarActionsDecls emits the .Actions() surface for datastar-framed
// routes: the DatastarActions accessor struct with one URL-building method
// per route (every route in the templates variable, any representation), the
// DatastarAction value with its copy-on-write option setters, and the js()
// renderer producing a template.JS backend-action expression that is safe by
// construction — each interpolated path segment is percent-encoded at build
// time and the whole URL (and every string option value) is JS-string escaped
// at render time. Inputs are taken raw: callers must not pre-escape, and no
// method accepts raw JS.
func datastarActionsDecls(file *File, config RoutesFileConfiguration, defs []muxt.Definition) ([]ast.Decl, error) {
	decls := []ast.Decl{
		// type DatastarActions struct { pathsPrefix string }
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
			Name: ast.NewIdent(datastarActionsTypeIdent),
			Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string")},
			}}},
		}}},
		datastarActionType(),
	}
	for i := range defs {
		decl, err := datastarActionFunc(file, &defs[i])
		if err != nil {
			return nil, err
		}
		decls = append(decls, decl)
	}
	for _, opt := range datastarActionOptions {
		decls = append(decls, datastarActionSetter(opt.Method, opt.Field, opt.GoType))
	}
	decls = append(decls, datastarActionURLMethod(), datastarActionJSMethod(file), datastarActionOptionsObjectMethod(file))
	return decls, nil
}

// datastarActionURLMethod exposes the percent-encoded URL as a plain string:
// useful for hand-written expressions and tests, and safe because a string
// stays under normal contextual autoescaping (this is not a raw-JS bypass).
func datastarActionURLMethod() *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent(datastarActionValueReceiver)},
			Type:  ast.NewIdent(datastarActionTypeIdent),
		}}},
		Name: ast.NewIdent("URL"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.SelectorExpr{X: ast.NewIdent(datastarActionValueReceiver), Sel: ast.NewIdent(datastarActionFieldURL)},
		}}}},
	}
}

// datastarActionType declares the action value: the verb and pre-encoded URL
// plus one pointer field per option (nil means the option is not rendered).
func datastarActionType() *ast.GenDecl {
	fields := []*ast.Field{{
		Names: []*ast.Ident{ast.NewIdent(datastarActionFieldMethod), ast.NewIdent(datastarActionFieldURL)},
		Type:  ast.NewIdent("string"),
	}}
	for _, opt := range datastarActionOptions {
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(opt.Field)},
			Type:  &ast.StarExpr{X: ast.NewIdent(opt.GoType)},
		})
	}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: ast.NewIdent(datastarActionTypeIdent),
		Type: &ast.StructType{Fields: &ast.FieldList{List: fields}},
	}}}
}

// datastarActionSetter builds the copy-on-write fluent setter:
//
//	func (action DatastarAction) Name(value T) DatastarAction {
//		action.field = &value
//		return action
//	}
func datastarActionSetter(methodName, field, goType string) *ast.FuncDecl {
	const paramName = "value"
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent(datastarActionValueReceiver)},
			Type:  ast.NewIdent(datastarActionTypeIdent),
		}}},
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(paramName)}, Type: ast.NewIdent(goType)}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(datastarActionTypeIdent)}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(datastarActionValueReceiver), Sel: ast.NewIdent(field)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(paramName)}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(datastarActionValueReceiver)}},
		}},
	}
}

// datastarActionJSMethod builds the render path:
//
//	func (action DatastarAction) js() template.JS {
//		return template.JS("@" + action.method + "('" + template.JSEscapeString(action.url) + "'" + action.optionsObject() + ")")
//	}
func datastarActionJSMethod(file *File) *ast.FuncDecl {
	concat := func(parts ...ast.Expr) ast.Expr {
		expr := parts[0]
		for _, part := range parts[1:] {
			expr = &ast.BinaryExpr{X: expr, Op: token.ADD, Y: part}
		}
		return expr
	}
	actionSel := func(field string) ast.Expr {
		return &ast.SelectorExpr{X: ast.NewIdent(datastarActionValueReceiver), Sel: ast.NewIdent(field)}
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent(datastarActionValueReceiver)},
			Type:  ast.NewIdent(datastarActionTypeIdent),
		}}},
		Name: ast.NewIdent("js"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: astgen.ExportedIdentifier(file, "template", "html/template", "JS")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{
			Fun: astgen.ExportedIdentifier(file, "template", "html/template", "JS"),
			Args: []ast.Expr{concat(
				astgen.String("@"),
				actionSel(datastarActionFieldMethod),
				astgen.String("('"),
				astgen.Call(file, "template", "html/template", "JSEscapeString", actionSel(datastarActionFieldURL)),
				astgen.String("'"),
				&ast.CallExpr{Fun: actionSel("optionsObject")},
				astgen.String(")"),
			)},
		}}}}},
	}
}

// datastarActionOptionsObjectMethod builds optionsObject, which renders the
// set options as a Datastar options object literal (", {k: v, ...}") in
// declaration order, or "" when no option is set. String values are JS-string
// escaped and single-quoted; numeric and bool values render via strconv.
func datastarActionOptionsObjectMethod(file *File) *ast.FuncDecl {
	const (
		partsIdent = "parts"
	)
	actionSel := func(field string) ast.Expr {
		return &ast.SelectorExpr{X: ast.NewIdent(datastarActionValueReceiver), Sel: ast.NewIdent(field)}
	}
	deref := func(field string) ast.Expr { return &ast.StarExpr{X: actionSel(field)} }
	body := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ast.NewIdent(partsIdent)},
			Type:  &ast.ArrayType{Elt: ast.NewIdent("string")},
		}}}},
	}
	for _, opt := range datastarActionOptions {
		var valueExpr ast.Expr
		switch {
		case opt.IsJSStr:
			valueExpr = &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  astgen.String("'"),
					Op: token.ADD,
					Y:  astgen.Call(file, "template", "html/template", "JSEscapeString", deref(opt.Field)),
				},
				Op: token.ADD,
				Y:  astgen.String("'"),
			}
		case opt.GoType == "bool":
			valueExpr = astgen.Call(file, "strconv", "strconv", "FormatBool", deref(opt.Field))
		case opt.GoType == "int":
			valueExpr = astgen.Call(file, "strconv", "strconv", "Itoa", deref(opt.Field))
		case opt.GoType == "float64":
			valueExpr = astgen.Call(file, "strconv", "strconv", "FormatFloat", deref(opt.Field),
				&ast.BasicLit{Kind: token.CHAR, Value: `'g'`}, astgen.Int(-1), astgen.Int(64))
		}
		body = append(body, &ast.IfStmt{
			Cond: &ast.BinaryExpr{X: actionSel(opt.Field), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(partsIdent)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{
					ast.NewIdent(partsIdent),
					&ast.BinaryExpr{X: astgen.String(opt.Key + ": "), Op: token.ADD, Y: valueExpr},
				}}},
			}}},
		})
	}
	body = append(body,
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: astgen.CallBuiltinLen(ast.NewIdent(partsIdent)), Op: token.EQL, Y: astgen.Int(0)},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{astgen.String("")}}}},
		},
		&ast.ReturnStmt{Results: []ast.Expr{&ast.BinaryExpr{
			X: &ast.BinaryExpr{
				X:  astgen.String(", {"),
				Op: token.ADD,
				Y:  astgen.Call(file, "strings", "strings", "Join", ast.NewIdent(partsIdent), astgen.String(", ")),
			},
			Op: token.ADD,
			Y:  astgen.String("}"),
		}}},
	)
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent(datastarActionValueReceiver)},
			Type:  ast.NewIdent(datastarActionTypeIdent),
		}}},
		Name: ast.NewIdent("optionsObject"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}

// datastarActionAccessorDecls builds the .Actions() accessor and the .JS
// render helper for one datastar template-data type. sseShaped selects the
// event template data's receiver shape.
func datastarActionAccessorDecls(file *File, typeIdent string, sseShaped bool) []ast.Decl {
	receiverName, recv := templateDataReceiverName, templateDataMethodReceiver
	if sseShaped {
		receiverName, recv = sseTemplateDataReceiverName, sseTemplateDataMethodReceiver
	}
	return []ast.Decl{
		&ast.FuncDecl{
			Recv: recv(typeIdent),
			Name: ast.NewIdent("Actions"),
			Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(datastarActionsTypeIdent)}}}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{
				Type: ast.NewIdent(datastarActionsTypeIdent),
				Elts: []ast.Expr{&ast.KeyValueExpr{
					Key:   ast.NewIdent(pathPrefixPathsStructFieldName),
					Value: &ast.SelectorExpr{X: ast.NewIdent(receiverName), Sel: ast.NewIdent(pathPrefixPathsStructFieldName)},
				}},
			}}}}},
		},
		&ast.FuncDecl{
			Recv: recv(typeIdent),
			Name: ast.NewIdent("JS"),
			Type: &ast.FuncType{
				Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(datastarActionValueReceiver)}, Type: ast.NewIdent(datastarActionTypeIdent)}}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: astgen.ExportedIdentifier(file, "template", "html/template", "JS")}}},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{X: ast.NewIdent(datastarActionValueReceiver), Sel: ast.NewIdent("js")},
			}}}}},
		},
	}
}

// datastarActionVerb maps a route's HTTP method to its Datastar backend
// action name; routes without a method act as GET.
func datastarActionVerb(def *muxt.Definition) string {
	method := def.HTTPMethod()
	if method == "" {
		method = "GET"
	}
	return strings.ToLower(method)
}

// datastarActionFunc builds the per-route URL-constructing method, mirroring
// the route's .Path() method with one addition: every interpolated path
// segment is percent-encoded with url.PathEscape, so a hostile value cannot
// add path segments or a query string and the produced URL round-trips
// through request.PathValue.
func datastarActionFunc(file *File, def *muxt.Definition) (*ast.FuncDecl, error) {
	encodingPkg, ok := file.Types("encoding")
	if !ok {
		return nil, fmt.Errorf(`the "encoding" package must be loaded`)
	}
	textMarshalerInterface := encodingPkg.Scope().Lookup("TextMarshaler").Type().Underlying().(*types.Interface)

	ident, err := def.ExportedPathIdentifier()
	if err != nil {
		return nil, err
	}

	actionResult := func(urlExpr ast.Expr) ast.Expr {
		return &ast.CompositeLit{Type: ast.NewIdent(datastarActionTypeIdent), Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent(datastarActionFieldMethod), Value: astgen.String(datastarActionVerb(def))},
			&ast.KeyValueExpr{Key: ast.NewIdent(datastarActionFieldURL), Value: urlExpr},
		}}
	}

	method := &ast.FuncDecl{
		Name: ast.NewIdent(ident),
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent(datastarActionReceiverName)},
			Type:  ast.NewIdent(datastarActionsTypeIdent),
		}}},
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: nil},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(datastarActionTypeIdent)}}},
		},
		Body: &ast.BlockStmt{List: nil},
	}

	prefixOr := astgen.Call(file, "cmp", "cmp", "Or",
		&ast.SelectorExpr{X: ast.NewIdent(datastarActionReceiverName), Sel: ast.NewIdent(pathPrefixPathsStructFieldName)},
		astgen.String("/"),
	)

	if def.Path() == "/" || def.Path() == "/{$}" {
		method.Body.List = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			actionResult(astgen.Call(file, "path", "path", "Join", prefixOr)),
		}}}
		return method, nil
	}

	templatePath, hasDollarSuffix := strings.CutSuffix(def.Path(), "{$}")
	segmentStrings := strings.Split(templatePath, "/")
	var (
		fields             []*ast.Field
		last               types.Type
		identIndex         = 0
		segmentIdentifiers = def.PathValueIdentifiers()
		hasErrorResult     = false
	)
	segmentExpressions := []ast.Expr{prefixOr}
	for si, segment := range segmentStrings {
		if len(segment) < 1 {
			continue
		}
		if segment[0] != '{' || segment[len(segment)-1] != '}' {
			if len(segmentExpressions) > 0 {
				prev := segmentExpressions[len(segmentExpressions)-1]
				if prevBasic, ok := prev.(*ast.BasicLit); ok {
					prevVal, _ := strconv.Unquote(prevBasic.Value)
					prevBasic.Value = strconv.Quote(prevVal + "/" + segment)
					continue
				}
			}
			segmentExpressions = append(segmentExpressions, astgen.String(segment))
			continue
		}

		paramIdent := segmentIdentifiers[identIndex]
		pathValueType, ok := def.ArgumentType(paramIdent)
		identIndex++
		if !ok {
			pathValueType = types.Universe.Lookup("string").Type()
		}
		tpNode, err := file.TypeASTExpression(pathValueType)
		if err != nil {
			return nil, err
		}
		if last != nil && len(fields) > 0 && types.Identical(last, pathValueType) {
			fields[len(fields)-1].Names = append(fields[len(fields)-1].Names, ast.NewIdent(paramIdent))
		} else {
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(paramIdent)}, Type: tpNode})
			last = pathValueType
		}

		pathEscape := func(x ast.Expr) ast.Expr {
			return astgen.Call(file, "url", "net/url", "PathEscape", x)
		}

		if types.Implements(pathValueType, textMarshalerInterface) {
			hasErrorResult = true
			if len(method.Type.Results.List) == 1 {
				method.Type.Results.List = append(method.Type.Results.List, &ast.Field{Type: ast.NewIdent("error")})
			}
			segmentIdent := fmt.Sprintf("segment%d", si)
			method.Body.List = append(method.Body.List,
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(segmentIdent), ast.NewIdent(errIdent)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(paramIdent), Sel: ast.NewIdent("MarshalText")}}},
				},
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
						&ast.CompositeLit{Type: ast.NewIdent(datastarActionTypeIdent)},
						astgen.Call(file, "fmt", "fmt", "Errorf",
							astgen.String(fmt.Sprintf("failed to marshal path value {%s} (segment %d) in %s: %%w", paramIdent, si, def.Path())),
							ast.NewIdent(errIdent),
						),
					}}}},
				},
			)
			segmentExpressions = append(segmentExpressions, pathEscape(&ast.CallExpr{
				Fun:  ast.NewIdent("string"),
				Args: []ast.Expr{ast.NewIdent(segmentIdent)},
			}))
			continue
		}

		basicType, ok := pathValueType.Underlying().(*types.Basic)
		if !ok {
			return nil, fmt.Errorf("unsupported type %s for path parameters: %s", astgen.Format(tpNode), paramIdent)
		}
		exp, err := astgen.ConvertToString(file, ast.NewIdent(paramIdent), basicType.Kind())
		if err != nil {
			return nil, fmt.Errorf("failed to encode variable %s: %v", paramIdent, err)
		}
		segmentExpressions = append(segmentExpressions, pathEscape(exp))
	}

	urlExpr := ast.Expr(&ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(file.Import("path", "path")), Sel: ast.NewIdent("Join")},
		Args: segmentExpressions,
	})
	if hasDollarSuffix {
		urlExpr = &ast.BinaryExpr{X: urlExpr, Op: token.ADD, Y: astgen.String("/")}
	}

	if hasErrorResult {
		method.Body.List = append(method.Body.List, &ast.ReturnStmt{Results: []ast.Expr{actionResult(urlExpr), astgen.Nil()}})
	} else {
		method.Body.List = append(method.Body.List, &ast.ReturnStmt{Results: []ast.Expr{actionResult(urlExpr)}})
	}

	method.Type.Params.List = fields
	return method, nil
}
