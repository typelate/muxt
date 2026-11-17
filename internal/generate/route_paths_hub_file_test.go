package generate

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
)

func hubTestPackage(t *testing.T, namespace, goSource, templates string) RoutePathsPackage {
	t.Helper()
	config := testConfig()
	config.ReceiverType = "T"
	pkg, receiver := testSource(t, goSource, "T", templates)
	defs, err := muxt.ResolveDefinitions(pkg, receiver, fake.NewChecker().Fake())
	require.NoError(t, err)
	return RoutePathsPackage{Namespace: namespace, Config: config, Pkg: pkg, Defs: defs}
}

// TestRoutePathsFileDeclaresStaticAssetsPath gives a package a static assets
// path and reads its namespace on the hub as Go.
func TestRoutePathsFileDeclaresStaticAssetsPath(t *testing.T) {
	blog := hubTestPackage(t, "Blog", `package server

type T struct{}

func (T) GetArticle(id int) string { return "" }
`, `{{define "GET /article/{id} GetArticle(id)"}}{{end}}`)
	blog.Config.StaticAssetsDirectory = "assets"
	blog.Config.StaticAssetsPath = "/static"

	file, err := RoutePathsFile(RoutePaths{ImportPath: "example.com/hub", PackageName: "hub", Packages: []RoutePathsPackage{blog}})
	require.NoError(t, err)
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "route_paths.go", file.Content, 0)
	require.NoError(t, err)
	pkg, err := (&types.Config{Importer: importer.ForCompiler(fset, "source", nil)}).Check("example.com/hub", fset, []*ast.File{parsed}, nil)
	require.NoError(t, err, file.Content)

	bare := func(*types.Package) string { return "" }
	for _, tc := range []struct{ typeName, want string }{
		{"BlogTemplateRoutePaths", "func() string"},
		{"BlogTemplateRouteBuilder", "func() TemplateRoute"},
	} {
		obj := pkg.Scope().Lookup(tc.typeName)
		require.NotNil(t, obj, tc.typeName)
		m, _, _ := types.LookupFieldOrMethod(obj.Type(), false, pkg, "ReadStatic")
		if assert.NotNil(t, m, "%s.ReadStatic", tc.typeName) {
			assert.Equal(t, tc.want, types.TypeString(m.Type(), bare), "%s.ReadStatic", tc.typeName)
		}
	}
}

// TestRoutePathsFileDeclaresEachPackagesHelpers generates the hub file for two
// packages and reads the result as Go.
func TestRoutePathsFileDeclaresEachPackagesHelpers(t *testing.T) {
	admin := hubTestPackage(t, "Admin", `package server

type T struct{}

func (T) Dashboard() string { return "" }
`, `{{define "GET /admin Dashboard()"}}{{end}}`)
	blog := hubTestPackage(t, "Blog", `package server

type T struct{}

func (T) GetArticle(id int) string { return "" }
`, `{{define "GET /article/{id} GetArticle(id)"}}{{end}}`)

	generated := func(packages ...RoutePathsPackage) (string, *types.Package) {
		t.Helper()
		file, err := RoutePathsFile(RoutePaths{ImportPath: "example.com/hub", PackageName: "hub", Packages: packages})
		require.NoError(t, err)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, "route_paths.go", file.Content, 0)
		require.NoError(t, err)
		pkg, err := (&types.Config{Importer: importer.ForCompiler(fset, "source", nil)}).Check("example.com/hub", fset, []*ast.File{parsed}, nil)
		require.NoError(t, err, file.Content)
		return file.Content, pkg
	}
	content, pkg := generated(admin, blog)

	bare := func(*types.Package) string { return "" }
	signature := func(typeName, method string) string {
		t.Helper()
		obj := pkg.Scope().Lookup(typeName)
		if obj == nil {
			return "<no type " + typeName + ">"
		}
		m, _, _ := types.LookupFieldOrMethod(obj.Type(), false, pkg, method)
		if m == nil {
			return "<no method " + method + ">"
		}
		return types.TypeString(m.Type(), bare)
	}
	for _, tc := range []struct{ typeName, method, want string }{
		{"TemplateRoutePaths", "Admin", "func() AdminTemplateRoutePaths"},
		{"TemplateRoutePaths", "Blog", "func() BlogTemplateRoutePaths"},
		{"TemplateRouteBuilder", "Admin", "func() AdminTemplateRouteBuilder"},
		{"TemplateRouteBuilder", "Blog", "func() BlogTemplateRouteBuilder"},
		{"BlogTemplateRoutePaths", "GetArticle", "func(idPathParam int) string"},
		{"BlogTemplateRouteBuilder", "GetArticle", "func(idPathParam int) TemplateRoute"},
		{"AdminTemplateRoutePaths", "Dashboard", "func() string"},
		{"AdminTemplateRouteBuilder", "Dashboard", "func() TemplateRoute"},
	} {
		assert.Equal(t, tc.want, signature(tc.typeName, tc.method), "%s.%s", tc.typeName, tc.method)
	}
	for name, want := range map[string]string{
		"NewTemplateRoutePaths":   "func(pathsPrefix string) TemplateRoutePaths",
		"NewTemplateRouteBuilder": "func(pathsPrefix string) TemplateRouteBuilder",
	} {
		fn, _ := pkg.Scope().Lookup(name).(*types.Func)
		if assert.NotNil(t, fn, "%s declared", name) {
			assert.Equal(t, want, types.TypeString(fn.Type(), bare), name)
		}
	}
	routeTypes := 0
	for _, name := range pkg.Scope().Names() {
		if name == "TemplateRoute" {
			routeTypes++
		}
	}
	assert.Equal(t, 1, routeTypes, "TemplateRoute declarations")

	reordered, _ := generated(blog, admin)
	assert.Equal(t, content, reordered, "output with packages in the other order")
}

// TestRoutePathsFileUsesConfiguredTypeNames names the hub's types and reads
// the generated file as Go.
func TestRoutePathsFileUsesConfiguredTypeNames(t *testing.T) {
	blog := hubTestPackage(t, "Blog", `package server

type T struct{}

func (T) GetArticle(id int) string { return "" }
`, `{{define "GET /article/{id} GetArticle(id)"}}{{end}}`)

	for _, tc := range []struct {
		name        string
		names       HubTypeNames
		declared    []string
		notDeclared []string
	}{
		{
			name: "all five",
			names: HubTypeNames{
				Paths: "Paths", RouteBuilder: "Routes", Route: "Route",
				PackagePathsSuffix: "Paths", PackageRouteBuilderSuffix: "Routes",
			},
			declared:    []string{"Paths", "Routes", "Route", "NewPaths", "NewRoutes", "BlogPaths", "BlogRoutes"},
			notDeclared: []string{"TemplateRoutePaths", "TemplateRouteBuilder", "TemplateRoute", "BlogTemplateRoutePaths"},
		},
		{
			name:        "only the package paths suffix",
			names:       HubTypeNames{PackagePathsSuffix: "Paths"},
			declared:    []string{"TemplateRoutePaths", "TemplateRouteBuilder", "TemplateRoute", "BlogPaths", "BlogTemplateRouteBuilder"},
			notDeclared: []string{"BlogTemplateRoutePaths"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := RoutePathsFile(RoutePaths{ImportPath: "example.com/hub", PackageName: "hub", Names: tc.names, Packages: []RoutePathsPackage{blog}})
			require.NoError(t, err)
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, "route_paths.go", file.Content, 0)
			require.NoError(t, err)
			pkg, err := (&types.Config{Importer: importer.ForCompiler(fset, "source", nil)}).Check("example.com/hub", fset, []*ast.File{parsed}, nil)
			require.NoError(t, err, file.Content)
			for _, name := range tc.declared {
				assert.NotNil(t, pkg.Scope().Lookup(name), "%s declared", name)
			}
			for _, name := range tc.notDeclared {
				assert.Nil(t, pkg.Scope().Lookup(name), "%s declared", name)
			}
		})
	}
}
