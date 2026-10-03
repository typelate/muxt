package generate

import (
	"cmp"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

const (
	routeBuilderReceiverName   = "routes"
	routePathsReceiverName     = "routePaths"
	routeFieldName             = "Route"
	routeReceiverName          = "route"
	routeMethodFieldName       = "method"
	routePathFieldName         = "path"
	escapePathSegmentFuncName  = "escapePathSegment"
	escapePathSegmentsFuncName = "escapePathSegments"
)

// routeTypeName is the name of the type the route path methods return. The
// zero value names the default.
func routeTypeName(config RoutesFileConfiguration) string {
	return cmp.Or(config.TemplateRouteTypeName, DefaultTemplateRouteTypeName)
}

// routeBuilderTypeName is the name of the type of the Route field of the paths
// type. Its methods return the route a path method returns the path of.
func routeBuilderTypeName(config RoutesFileConfiguration) string {
	return routeTypeName(config) + "Builder"
}

// pathsLiteral is the paths value for a path prefix expression.
func pathsLiteral(config RoutesFileConfiguration, prefix ast.Expr) *ast.CompositeLit {
	return &ast.CompositeLit{Type: ast.NewIdent(config.TemplateRoutePathsTypeName), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(routeFieldName), Value: &ast.CompositeLit{
			Type: ast.NewIdent(routeBuilderTypeName(config)),
			Elts: []ast.Expr{&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: prefix}},
		}},
	}}
}

// routeTypeDecls emits the type a route path method returns. It renders as the
// path, so a template that prints it gets the same text the method used to
// return, and it knows the HTTP method of the route it names.
func routeTypeDecls(config RoutesFileConfiguration) []ast.Decl {
	typeName := routeTypeName(config)
	accessor := func(name, field string) *ast.FuncDecl {
		return &ast.FuncDecl{
			Name: ast.NewIdent(name),
			Recv: &ast.FieldList{List: []*ast.Field{param(ast.NewIdent(typeName), routeReceiverName)}},
			Type: &ast.FuncType{Params: &ast.FieldList{}, Results: fieldList(results(ast.NewIdent("string")))},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(selector(routeReceiverName, field))}},
		}
	}
	return []ast.Decl{
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{
			&ast.TypeSpec{Name: ast.NewIdent(typeName), Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(routeMethodFieldName)}, Type: ast.NewIdent("string")},
				{Names: []*ast.Ident{ast.NewIdent(routePathFieldName)}, Type: ast.NewIdent("string")},
			}}}},
		}},
		accessor("String", routePathFieldName),
		accessor("Method", routeMethodFieldName),
	}
}

// routeLiteral is the route a path method returns for def with the given path.
func routeLiteral(config RoutesFileConfiguration, def *muxt.Definition, path ast.Expr) ast.Expr {
	return &ast.CompositeLit{
		Type: ast.NewIdent(routeTypeName(config)),
		Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent(routeMethodFieldName), Value: astgen.String(def.HTTPMethod())},
			&ast.KeyValueExpr{Key: ast.NewIdent(routePathFieldName), Value: path},
		},
	}
}

func routePathTypeAndMethods(imports *File, config RoutesFileConfiguration, defs []muxt.Definition) ([]ast.Decl, error) {
	decls := []ast.Decl{
		&ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{
				&ast.TypeSpec{Name: ast.NewIdent(config.TemplateRoutePathsTypeName), Type: &ast.StructType{Fields: &ast.FieldList{
					List: []*ast.Field{
						{Names: []*ast.Ident{ast.NewIdent(routeFieldName)}, Type: ast.NewIdent(routeBuilderTypeName(config))},
					},
				}}},
			},
		},
		&ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{
				&ast.TypeSpec{Name: ast.NewIdent(routeBuilderTypeName(config)), Type: &ast.StructType{Fields: &ast.FieldList{
					List: []*ast.Field{
						{Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string")},
					},
				}}},
			},
		},
	}
	decls = append(decls, routeTypeDecls(config)...)
	if err := muxt.CheckPathMethodCollisions(defs); err != nil {
		return nil, err
	}
	var used escaperUse
	for _, t := range defs {
		decl, escapers, err := routePathFunc(imports, config, &t)
		if err != nil {
			return nil, err
		}
		used.segment = used.segment || escapers.segment
		used.segments = used.segments || escapers.segments
		decls = append(decls, decl, routePathWrapper(config, decl))
	}
	// escapePathSegments calls escapePathSegment, so needing the former
	// implies emitting both.
	if used.segment || used.segments {
		decls = append(decls, escapePathSegmentMethod(imports, config))
	}
	if used.segments {
		decls = append(decls, escapePathSegmentsMethod(imports, config))
	}
	return decls, nil
}

// escaperUse records which generated escaper methods a route path method calls.
type escaperUse struct{ segment, segments bool }

func routeBuilderMethod(config RoutesFileConfiguration, name string, params, results []*ast.Field, body ...ast.Stmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: ast.NewIdent(name),
		Recv: &ast.FieldList{List: []*ast.Field{param(ast.NewIdent(routeBuilderTypeName(config)), routeBuilderReceiverName)}},
		Type: &ast.FuncType{Params: &ast.FieldList{List: params}, Results: fieldList(results)},
		Body: &ast.BlockStmt{List: body},
	}
}

func routePathFunc(file *File, config RoutesFileConfiguration, def *muxt.Definition) (*ast.FuncDecl, escaperUse, error) {
	ident, err := def.ExportedPathIdentifier()
	if err != nil {
		return nil, escaperUse{}, err
	}
	routeType := ast.NewIdent(routeTypeName(config))
	if def.IsIndex() {
		var indexPath ast.Expr = astgen.String("/")
		if config.PathPrefix {
			indexPath = astgen.Call(file, "path", "path", "Join", pathPrefixOrRoot(file))
		}
		return routeBuilderMethod(config, ident, nil, results(routeType), returnExprs(routeLiteral(config, def, indexPath))), escaperUse{}, nil
	}

	b := &routePathBuilder{file: file, config: config, def: def, segments: []ast.Expr{pathPrefixOrRoot(file)}}
	for i, segment := range def.Segments {
		// The segment number counts as the path splits on "/", with the empty
		// segment before the leading "/" at 0.
		if err := b.addSegment(i+1, segment); err != nil {
			return nil, escaperUse{}, err
		}
	}
	b.flushLiteral()

	var joined ast.Expr = astgen.Call(file, "path", "path", "Join", b.segments...)
	if def.HasPathEndWildcard() {
		joined = &ast.BinaryExpr{X: joined, Op: token.ADD, Y: astgen.String("/")}
	}
	returned, resultTypes := []ast.Expr{routeLiteral(config, def, joined)}, []ast.Expr{routeType}
	if b.returnsError {
		returned = append(returned, astgen.Nil())
		resultTypes = append(resultTypes, ast.NewIdent("error"))
	}
	body := append(b.statements, returnExprs(returned...))
	return routeBuilderMethod(config, ident, b.fields, results(resultTypes...), body...), b.escapers, nil
}

// routePathWrapper emits the path method that calls the route method builder
// is, so code that uses the path as a string keeps working:
//
//	func (routePaths TemplateRoutePaths) GetItem(id int) string {
//		return routePaths.Route.GetItem(id).String()
//	}
//
// A builder method that returns an error gets a wrapper that returns it too.
func routePathWrapper(config RoutesFileConfiguration, builder *ast.FuncDecl) *ast.FuncDecl {
	var args []ast.Expr
	for _, field := range builder.Type.Params.List {
		for _, name := range field.Names {
			args = append(args, ast.NewIdent(name.Name))
		}
	}
	call := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: selector(routePathsReceiverName, routeFieldName), Sel: ast.NewIdent(builder.Name.Name)},
		Args: args,
	}
	asString := func(route ast.Expr) ast.Expr {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: route, Sel: ast.NewIdent("String")}}
	}
	decl := &ast.FuncDecl{
		Name: ast.NewIdent(builder.Name.Name),
		Recv: &ast.FieldList{List: []*ast.Field{param(ast.NewIdent(config.TemplateRoutePathsTypeName), routePathsReceiverName)}},
		Type: &ast.FuncType{Params: builder.Type.Params},
	}
	if len(builder.Type.Results.List) == 1 {
		decl.Type.Results = fieldList(results(ast.NewIdent("string")))
		decl.Body = &ast.BlockStmt{List: []ast.Stmt{returnExprs(asString(call))}}
		return decl
	}
	decl.Type.Results = fieldList(results(ast.NewIdent("string"), ast.NewIdent("error")))
	decl.Body = &ast.BlockStmt{List: []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(routeReceiverName), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(astgen.String(""), ast.NewIdent(errIdent))}},
		},
		returnExprs(asString(ast.NewIdent(routeReceiverName)), astgen.Nil()),
	}}
	return decl
}

// pathPrefixOrRoot is cmp.Or(routes.pathsPrefix, "/").
func pathPrefixOrRoot(file *File) ast.Expr {
	return astgen.Call(file, "cmp", "cmp", "Or", selector(routeBuilderReceiverName, pathPrefixPathsStructFieldName), astgen.String("/"))
}

// routePathBuilder accumulates the parameters, statements and path segments of
// one route path method.
type routePathBuilder struct {
	file   *File
	config RoutesFileConfiguration
	def    *muxt.Definition

	fields     []*ast.Field
	lastType   source.Type
	statements []ast.Stmt
	segments   []ast.Expr
	// literals are consecutive literal segments not yet joined into one
	// string in segments.
	literals []string

	returnsError bool
	escapers     escaperUse
}

func (b *routePathBuilder) addSegment(number int, segment muxt.Segment) error {
	if segment.IsLiteral() {
		b.literals = append(b.literals, segment.Value())
		return nil
	}
	b.flushLiteral()

	ident := pathParamIdent(segment.Value())
	arg := segment.Argument()
	valueType := pathSegmentHelperType(arg)
	if err := b.declareParameter(ident, valueType); err != nil {
		return err
	}
	if arg != nil && !arg.Direct() && arg.TextMarshaler() {
		b.addMarshaledSegment(number, segment, ident)
		return nil
	}
	return b.addFormattedSegment(segment, ident, valueType)
}

func (b *routePathBuilder) flushLiteral() {
	if len(b.literals) == 0 {
		return
	}
	b.segments = append(b.segments, astgen.String(strings.Join(b.literals, "/")))
	b.literals = nil
}

// declareParameter adds ident to the method's parameters, sharing the previous
// field when it has the same type.
func (b *routePathBuilder) declareParameter(ident string, valueType source.Type) error {
	typeExpr, err := b.file.TypeExpr(valueType)
	if err != nil {
		return err
	}
	if !b.lastType.IsZero() && b.lastType.Identical(valueType) {
		last := b.fields[len(b.fields)-1]
		last.Names = append(last.Names, ast.NewIdent(ident))
		return nil
	}
	b.fields = append(b.fields, param(typeExpr, ident))
	b.lastType = valueType
	return nil
}

// addMarshaledSegment marshals the parameter with MarshalText, which is why
// the method then returns an error.
func (b *routePathBuilder) addMarshaledSegment(number int, segment muxt.Segment, ident string) {
	b.returnsError = true
	hash := sha1.Sum([]byte(b.def.Name()))
	segmentIdent := fmt.Sprintf("segment%d_%s", number, hex.EncodeToString(hash[:])[:8])
	message := fmt.Sprintf("failed to marshal path value {%s} (segment %d) in %s: %%w", segment.Value(), number, b.def.Path())
	b.statements = append(b.statements,
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(segmentIdent), ast.NewIdent(errIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: selector(ident, "MarshalText")}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(
				&ast.CompositeLit{Type: ast.NewIdent(routeTypeName(b.config))},
				astgen.Call(b.file, "fmt", "fmt", "Errorf", astgen.String(message), ast.NewIdent(errIdent)),
			)}},
		},
	)
	b.segments = append(b.segments, b.escaped(astgen.ConvertIdent("string", ast.NewIdent(segmentIdent)), segment))
}

func (b *routePathBuilder) addFormattedSegment(segment muxt.Segment, ident string, valueType source.Type) error {
	exp, err := astgen.ConvertToString(b.file, ast.NewIdent(ident), valueType)
	if err != nil {
		return fmt.Errorf("failed to encode variable %s: %v", ident, err)
	}
	if valueType.IsString() {
		exp = b.escaped(exp, segment)
	}
	b.segments = append(b.segments, exp)
	return nil
}

// escaped wraps value in the escaper for segment, and records that the
// generated file needs it.
func (b *routePathBuilder) escaped(value ast.Expr, segment muxt.Segment) ast.Expr {
	if segment.IsRemainder() {
		b.escapers.segments = true
		return escapedPathSegments(value)
	}
	b.escapers.segment = true
	return escapedPathSegment(value)
}

// pathParamIdent names the generated local for a path parameter. The suffix
// keeps any wildcard name from colliding with an identifier the generated
// code references: an import, another local, or a captured variable.
func pathParamIdent(name string) string {
	return name + "PathParam"
}

// pathSegmentHelperType is the type a wildcard segment's parameter takes in
// the generated route path method: string when the call does not link an
// argument to the segment or that argument is Direct (its parameter takes
// the raw request value as it is), otherwise the argument's parameter type.
func pathSegmentHelperType(arg *muxt.Argument) source.Type {
	if arg == nil || arg.Direct() {
		return source.StringType()
	}
	return arg.ParamType()
}

// escapedPathSegment wraps value in a call to the generated escapePathSegment
// method; the caller must arrange for escapePathSegmentMethod to be emitted.
func escapedPathSegment(value ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: selector(routeBuilderReceiverName, escapePathSegmentFuncName), Args: []ast.Expr{value}}
}

// escapedPathSegments wraps a trailing-wildcard value in a call to the
// generated escapePathSegments method; the caller must arrange for both
// escaper methods to be emitted.
func escapedPathSegments(value ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: selector(routeBuilderReceiverName, escapePathSegmentsFuncName), Args: []ast.Expr{value}}
}

// escapePathSegmentsMethod emits:
//
//	func (routePaths TemplateRoutePaths) escapePathSegments(value string) string {
//		segments := strings.Split(value, "/")
//		for i, segment := range segments {
//			segments[i] = routePaths.escapePathSegment(segment)
//		}
//		return strings.Join(segments, "/")
//	}
//
// A trailing {name...} wildcard names a multi-segment path suffix, so its "/"
// separators are meaningful and each segment between them is escaped alone.
func escapePathSegmentsMethod(file *File, config RoutesFileConfiguration) *ast.FuncDecl {
	const (
		valueIdent    = "value"
		segmentsIdent = "segments"
		indexIdent    = "i"
		segmentIdent  = "segment"
	)
	return routeBuilderMethod(config, escapePathSegmentsFuncName,
		[]*ast.Field{param(ast.NewIdent("string"), valueIdent)},
		results(ast.NewIdent("string")),
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(segmentsIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{astgen.Call(file, "strings", "strings", "Split", ast.NewIdent(valueIdent), astgen.String("/"))},
		},
		&ast.RangeStmt{
			Key:   ast.NewIdent(indexIdent),
			Value: ast.NewIdent(segmentIdent),
			Tok:   token.DEFINE,
			X:     ast.NewIdent(segmentsIdent),
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(segmentsIdent), Index: ast.NewIdent(indexIdent)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{escapedPathSegment(ast.NewIdent(segmentIdent))},
			}}},
		},
		returnExprs(astgen.Call(file, "strings", "strings", "Join", ast.NewIdent(segmentsIdent), astgen.String("/"))),
	)
}

// escapePathSegmentMethod emits:
//
//	func (routePaths TemplateRoutePaths) escapePathSegment(value string) string {
//		switch value {
//		case ".":
//			return "%2E"
//		case "..":
//			return "%2E%2E"
//		}
//		return url.PathEscape(value)
//	}
//
// url.PathEscape leaves "." and ".." unchanged ('.' is unreserved), but
// path.Join and request routing collapse dot segments, so a helper value of
// ".." would address a parent route. The percent-encoded forms survive both:
// http.ServeMux matches the escaped path and PathValue decodes them back.
func escapePathSegmentMethod(file *File, config RoutesFileConfiguration) *ast.FuncDecl {
	const valueIdent = "value"
	caseReturn := func(match, encoded string) ast.Stmt {
		return &ast.CaseClause{
			List: []ast.Expr{astgen.String(match)},
			Body: []ast.Stmt{returnExprs(astgen.String(encoded))},
		}
	}
	return routeBuilderMethod(config, escapePathSegmentFuncName,
		[]*ast.Field{param(ast.NewIdent("string"), valueIdent)},
		results(ast.NewIdent("string")),
		&ast.SwitchStmt{
			Tag:  ast.NewIdent(valueIdent),
			Body: &ast.BlockStmt{List: []ast.Stmt{caseReturn(".", "%2E"), caseReturn("..", "%2E%2E")}},
		},
		returnExprs(astgen.Call(file, "url", "net/url", "PathEscape", ast.NewIdent(valueIdent))),
	)
}
