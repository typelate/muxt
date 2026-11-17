package generate

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/ettle/strcase"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

// DefaultRoutePathsFileName is the base name of the hub file when none is set.
const DefaultRoutePathsFileName = "route_paths.go"

const (
	hubPathsTypeName   = "TemplateRoutePaths"
	hubBuilderTypeName = "TemplateRouteBuilder"
	hubRouteTypeName   = "TemplateRoute"
)

// RoutePaths is the hub file: the path helpers of every package that shares the hub.
type RoutePaths struct {
	ImportPath, PackageName string
	OutputFile              string // base name; empty means DefaultRoutePathsFileName
	Packages                []RoutePathsPackage
}

// RoutePathsPackage is one package whose path helpers the hub file declares.
type RoutePathsPackage struct {
	Namespace string
	Config    RoutesFileConfiguration
	Pkg       source.Package
	Defs      []muxt.Definition
}

// RoutePathsFile generates the hub file. The packages may be in any order.
func RoutePathsFile(r RoutePaths) (GeneratedFile, error) {
	packages := slices.SortedFunc(slices.Values(r.Packages), func(a, b RoutePathsPackage) int {
		return strings.Compare(a.Namespace, b.Namespace)
	})
	file := newFile(source.Package{Types: types.NewPackage(r.ImportPath, r.PackageName)})

	decls := hubBaseDecls(packages, hubPathsTypeName, func(ns string) ast.Expr {
		return &ast.CompositeLit{Type: ast.NewIdent(ns + hubPathsTypeName), Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent(routesPathsFieldName), Value: hubPrefixedLiteral(ns+hubBuilderTypeName, routePathsReceiverName)},
		}}
	})
	decls = append(decls, hubBaseDecls(packages, hubBuilderTypeName, func(ns string) ast.Expr {
		return hubPrefixedLiteral(ns+hubBuilderTypeName, routeBuilderReceiverName)
	})...)
	decls = append(decls, routeTypeDecls(pathTypeNames{Route: hubRouteTypeName})...)
	for _, p := range packages {
		names := pathTypeNames{Paths: p.Namespace + hubPathsTypeName, Builder: p.Namespace + hubBuilderTypeName, Route: hubRouteTypeName}
		packageDecls, err := routePathTypeAndMethods(file, p.Config, names, p.Defs, false)
		if err != nil {
			return GeneratedFile{}, fmt.Errorf("package %s: %w", p.Namespace, err)
		}
		decls = append(decls, packageDecls...)
	}
	if err := file.Err(); err != nil {
		return GeneratedFile{}, err
	}

	name := cmp.Or(r.OutputFile, DefaultRoutePathsFileName)
	content, err := formatFile(name, &ast.File{
		Name:  ast.NewIdent(r.PackageName),
		Decls: append([]ast.Decl{importDecl(file)}, decls...),
	})
	if err != nil {
		return GeneratedFile{}, err
	}
	return GeneratedFile{Path: name, Content: content}, nil
}

// hubPrefixedLiteral is typeName{pathsPrefix: receiver.pathsPrefix}.
func hubPrefixedLiteral(typeName, receiver string) ast.Expr {
	return &ast.CompositeLit{Type: ast.NewIdent(typeName), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: selector(receiver, pathPrefixPathsStructFieldName)},
	}}
}

// hubBaseDecls declares a base type holding the path prefix, its constructor,
// and one accessor per package. accessed builds the value an accessor returns
// for a namespace.
func hubBaseDecls(packages []RoutePathsPackage, typeName string, accessed func(namespace string) ast.Expr) []ast.Decl {
	receiver := routeBuilderReceiverName
	if typeName == hubPathsTypeName {
		receiver = routePathsReceiverName
	}
	decls := []ast.Decl{
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{
			&ast.TypeSpec{Name: ast.NewIdent(typeName), Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string")},
			}}}},
		}},
		&ast.FuncDecl{
			Name: ast.NewIdent("New" + typeName),
			Type: &ast.FuncType{Params: fieldList([]*ast.Field{param(ast.NewIdent("string"), pathPrefixPathsStructFieldName)}), Results: fieldList(results(ast.NewIdent(typeName)))},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(&ast.CompositeLit{Type: ast.NewIdent(typeName), Elts: []ast.Expr{
				&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: ast.NewIdent(pathPrefixPathsStructFieldName)},
			}})}},
		},
	}
	for _, p := range packages {
		decls = append(decls, &ast.FuncDecl{
			Name: ast.NewIdent(p.Namespace),
			Recv: &ast.FieldList{List: []*ast.Field{param(ast.NewIdent(typeName), receiver)}},
			Type: &ast.FuncType{Params: &ast.FieldList{}, Results: fieldList(results(ast.NewIdent(p.Namespace + typeName)))},
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(accessed(p.Namespace))}},
		})
	}
	return decls
}

// Hub is the package that holds the shared path helpers, as one routes file sees it.
type Hub struct {
	ImportPath  string
	PackageName string
	Namespace   string // of the package being generated
}

// CheckNamespaces reports the first two packages that would share a namespace.
func CheckNamespaces(importPaths []string, hubImportPath string) error {
	sorted := slices.Clone(importPaths)
	slices.Sort(sorted)
	seen := make(map[string]string, len(sorted))
	for _, importPath := range sorted {
		namespace, err := Namespace(importPath, hubImportPath)
		if err != nil {
			return err
		}
		if first, ok := seen[namespace]; ok {
			return &SameNamespaceError{Namespace: namespace, ImportPaths: [2]string{first, importPath}}
		}
		seen[namespace] = importPath
	}
	return nil
}

// SameNamespaceError is two packages whose routes would get one namespace.
type SameNamespaceError struct {
	Namespace   string
	ImportPaths [2]string // sorted
}

func (e *SameNamespaceError) Error() string {
	return fmt.Sprintf("packages %q and %q both get the route paths namespace %q; rename one of them",
		e.ImportPaths[0], e.ImportPaths[1], e.Namespace)
}

// Namespace returns the name a package's routes have on the hub's paths type.
func Namespace(importPath, hubImportPath string) (string, error) {
	rest, ok := strings.CutPrefix(importPath, hubImportPath+"/")
	if !ok {
		return "", fmt.Errorf("package %q is not below hub %q", importPath, hubImportPath)
	}
	var sb strings.Builder
	for _, elem := range strings.Split(rest, "/") {
		sb.WriteString(strcase.ToGoPascal(elem))
	}
	return sb.String(), nil
}
