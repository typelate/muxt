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

// HubTypeNames are the names of the types the hub file declares. A zero field takes its default.
type HubTypeNames struct {
	Paths, RouteBuilder, Route                    string
	PackagePathsSuffix, PackageRouteBuilderSuffix string
}

// PathsType is the name of the hub's paths type.
func (n HubTypeNames) PathsType() string { return cmp.Or(n.Paths, "TemplateRoutePaths") }

// RouteBuilderType is the name of the hub's route builder type.
func (n HubTypeNames) RouteBuilderType() string {
	return cmp.Or(n.RouteBuilder, "TemplateRouteBuilder")
}

// RouteType is the name of the hub's route type.
func (n HubTypeNames) RouteType() string { return cmp.Or(n.Route, "TemplateRoute") }

// PackagePathsType is the name of the paths type of the package with namespace.
func (n HubTypeNames) PackagePathsType(namespace string) string {
	return namespace + cmp.Or(n.PackagePathsSuffix, "TemplateRoutePaths")
}

// PackageRouteBuilderType is the name of the route builder type of the package with namespace.
func (n HubTypeNames) PackageRouteBuilderType(namespace string) string {
	return namespace + cmp.Or(n.PackageRouteBuilderSuffix, "TemplateRouteBuilder")
}

// hubConstructorName is the name of the function that makes the hub type typeName.
func hubConstructorName(typeName string) string { return "New" + typeName }

// RoutePaths is the hub file: the path helpers of every package that shares the hub.
type RoutePaths struct {
	ImportPath, PackageName string
	OutputFile              string // base name; empty means DefaultRoutePathsFileName
	Names                   HubTypeNames
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

	hub := r.Names
	decls := hubBaseDecls(packages, hub.PathsType(), routePathsReceiverName, func(ns string) ast.Expr {
		return &ast.CompositeLit{Type: ast.NewIdent(hub.PackagePathsType(ns)), Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent(routesPathsFieldName), Value: hubPrefixedLiteral(hub.PackageRouteBuilderType(ns), routePathsReceiverName)},
		}}
	}, hub.PackagePathsType)
	decls = append(decls, hubBaseDecls(packages, hub.RouteBuilderType(), routeBuilderReceiverName, func(ns string) ast.Expr {
		return hubPrefixedLiteral(hub.PackageRouteBuilderType(ns), routeBuilderReceiverName)
	}, hub.PackageRouteBuilderType)...)
	decls = append(decls, routeTypeDecls(pathTypeNames{Route: hub.RouteType()})...)
	for _, p := range packages {
		names := pathTypeNames{Paths: hub.PackagePathsType(p.Namespace), Builder: hub.PackageRouteBuilderType(p.Namespace), Route: hub.RouteType()}
		packageDecls, err := routePathTypeAndMethods(file, p.Config, names, p.Defs)
		if err != nil {
			return GeneratedFile{}, fmt.Errorf("package %s: %w", p.Namespace, err)
		}
		decls = append(decls, packageDecls...)
		if p.Config.StaticAssetsDirectory != "" {
			decls = append(decls, staticAssetsPathMethods(file, p.Config, names)...)
		}
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
// for a namespace, and accessedType names the type that is returned.
func hubBaseDecls(packages []RoutePathsPackage, typeName, receiver string, accessed func(namespace string) ast.Expr, accessedType func(namespace string) string) []ast.Decl {
	decls := []ast.Decl{
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{
			&ast.TypeSpec{Name: ast.NewIdent(typeName), Type: &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string")},
			}}}},
		}},
		&ast.FuncDecl{
			Name: ast.NewIdent(hubConstructorName(typeName)),
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
			Type: &ast.FuncType{Params: &ast.FieldList{}, Results: fieldList(results(ast.NewIdent(accessedType(p.Namespace))))},
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
	Names       HubTypeNames
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
