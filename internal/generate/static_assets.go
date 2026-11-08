package generate

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ettle/strcase"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	staticAssetsDirectoryFlag = "input-static-assets-directory"
	staticAssetsPathFlag      = "output-static-assets-path"

	// staticAssetsIdent is the local the routes function holds the
	// embedded directory's sub file system in.
	staticAssetsIdent = "staticAssets"

	// staticAssetsNameIdent is the local the static assets handler holds
	// the cleaned request path in.
	staticAssetsNameIdent = "name"

	staticAssetsVariableSuffix = "StaticAssets"
)

// StaticAssetsPrefix is the URL prefix --output-static-assets-path names:
// the path with a trailing "/" ("/static" and "/static/" are "/static/").
func StaticAssetsPrefix(path string) string {
	return strings.TrimSuffix(path, "/") + "/"
}

// StaticAssetsVariable is the unexported embed.FS variable the routes file
// declares: the routes function name in lowerCamel case plus StaticAssets
// ("templateRoutesStaticAssets"), so route sets sharing a package do not
// collide.
func StaticAssetsVariable(config RoutesFileConfiguration) string {
	return strcase.ToGoCamel(config.RoutesFunction) + staticAssetsVariableSuffix
}

// staticAssetsDirectory is --input-static-assets-directory as go:embed and
// fs.Sub take it: without a trailing "/".
func staticAssetsDirectory(config RoutesFileConfiguration) string {
	return strings.TrimSuffix(config.StaticAssetsDirectory, "/")
}

// checkStaticAssetsDirectory rejects a static assets directory the routes
// file cannot embed and serve. dir is the package directory the
// configured directory is relative to; defs are the routes the routes
// file registers beside it.
func checkStaticAssetsDirectory(dir string, config RoutesFileConfiguration, defs []muxt.Definition) error {
	assets := staticAssetsDirectory(config)
	full := filepath.Join(dir, filepath.FromSlash(assets))
	info, err := os.Lstat(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("--%s %s: no such directory in %s", staticAssetsDirectoryFlag, assets, dir)
		}
		return fmt.Errorf("--%s %s: %w", staticAssetsDirectoryFlag, assets, err)
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("--%s %s: is a symbolic link; name the directory itself", staticAssetsDirectoryFlag, assets)
	case !info.IsDir():
		return fmt.Errorf("--%s %s: not a directory", staticAssetsDirectoryFlag, assets)
	}
	embedded, err := embedsAFile(full)
	if err != nil {
		return fmt.Errorf("--%s %s: %w", staticAssetsDirectoryFlag, assets, err)
	}
	if !embedded {
		return fmt.Errorf("--%s %s: holds no file go:embed would include", staticAssetsDirectoryFlag, assets)
	}

	prefix := StaticAssetsPrefix(config.StaticAssetsPath)
	pattern := http.MethodGet + " " + prefix
	method := muxt.StaticAssetsPathIdentifier(prefix)
	for _, def := range defs {
		if def.Pattern() == pattern {
			return fmt.Errorf("--%s %s: template %q registers the same pattern", staticAssetsPathFlag, prefix, def.Name())
		}
		if matchesStaticAssetsRequests(def, prefix) {
			return fmt.Errorf("--%s %s: template %q registers a pattern that matches the same requests as the static assets route", staticAssetsPathFlag, prefix, def.Name())
		}
	}
	for _, def := range defs {
		ident, err := def.ExportedPathIdentifier()
		if err != nil {
			return err
		}
		if ident == method {
			return fmt.Errorf("--%s %s: path method %s is also the path method of template %q", staticAssetsPathFlag, prefix, method, def.Name())
		}
	}
	return nil
}

// matchesStaticAssetsRequests reports whether def's pattern is
// "GET <prefix>{name...}": it matches exactly the requests "GET <prefix>"
// does, so neither is more specific and http.ServeMux panics when the
// second is registered. A pattern with a host, another method, no method,
// or more literal segments is more or less specific and registers beside
// it.
func matchesStaticAssetsRequests(def muxt.Definition, prefix string) bool {
	if def.HTTPMethod() != http.MethodGet || def.Host() != "" {
		return false
	}
	rest, ok := strings.CutPrefix(def.Path(), prefix)
	if !ok {
		return false
	}
	name, ok := strings.CutPrefix(rest, "{")
	if !ok {
		return false
	}
	name, ok = strings.CutSuffix(name, "...}")
	return ok && name != "" && !strings.ContainsAny(name, "/{}")
}

// errFoundEmbeddedFile stops the walk in embedsAFile at the first file.
var errFoundEmbeddedFile = errors.New("found a file go:embed includes")

// embedsAFile reports whether //go:embed of the directory includes a file:
// a regular file none of whose path elements below the directory start
// with "." or "_".
func embedsAFile(dir string) (bool, error) {
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if name := entry.Name(); strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			return errFoundEmbeddedFile
		}
		return nil
	})
	if errors.Is(err, errFoundEmbeddedFile) {
		return true, nil
	}
	return false, err
}

// staticAssetsDecl declares the variable the static assets directory is
// embedded in:
//
//	//go:embed assets
//	var templateRoutesStaticAssets embed.FS
func staticAssetsDecl(file *File, config RoutesFileConfiguration) *ast.GenDecl {
	return &ast.GenDecl{
		Doc: &ast.CommentGroup{List: []*ast.Comment{{Text: "//go:embed " + staticAssetsDirectory(config)}}},
		Tok: token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ast.NewIdent(StaticAssetsVariable(config))},
			Type:  astgen.ExportedIdentifier(file, "", "embed", "FS"),
		}},
	}
}

// staticAssetsStatements serve the embedded directory under the prefix:
//
//	staticAssets, err := fs.Sub(templateRoutesStaticAssets, "assets")
//	if err != nil {
//		panic(fmt.Errorf("muxt: static assets directory %q: %w", "assets", err))
//	}
//	mux.Handle("GET /static/", http.StripPrefix("/static/", http.HandlerFunc(...)))
//
// where the handler is staticAssetsHandler. fs.Sub fails only for a path fs.ValidPath rejects, which muxt generate
// checked, so the panic is unreachable and the routes function keeps its
// signature. Under a path prefix the pattern's path and the stripped
// prefix are both joined onto it, and with a middleware parameter the
// handler is wrapped by it.
func staticAssetsStatements(file *File, config RoutesFileConfiguration) []ast.Stmt {
	prefix := StaticAssetsPrefix(config.StaticAssetsPath)
	var (
		pattern  ast.Expr = astgen.String(http.MethodGet + " " + prefix)
		stripped ast.Expr = astgen.String(prefix)
	)
	if config.PathPrefix {
		pattern = concatenation(append([]ast.Expr{astgen.String(http.MethodGet + " ")}, prefixedPath(file, ast.NewIdent(pathPrefixPathsStructFieldName), prefix)...)...)
		stripped = concatenation(prefixedPath(file, ast.NewIdent(pathPrefixPathsStructFieldName), prefix)...)
	}
	var handler ast.Expr = astgen.Call(file, "http", "net/http", "StripPrefix", stripped,
		staticAssetsHandler(file))
	if config.Middleware {
		handler = &ast.CallExpr{Fun: ast.NewIdent(middlewareParamName), Args: []ast.Expr{handler}}
	}
	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(staticAssetsIdent), ast.NewIdent(errIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{astgen.Call(file, "fs", "io/fs", "Sub", ast.NewIdent(StaticAssetsVariable(config)), astgen.String(staticAssetsDirectory(config)))},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: astgen.CallBuiltin("panic",
				astgen.Call(file, "fmt", "fmt", "Errorf", astgen.String("muxt: static assets directory %q: %w"), astgen.String(staticAssetsDirectory(config)), ast.NewIdent(errIdent)),
			)}}},
		},
		&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(muxVarIdent), Sel: ast.NewIdent(httpHandleIdent)},
			Args: []ast.Expr{pattern, handler},
		}},
	}
}

// staticAssetsHandler is http.FileServerFS that answers 404 for a
// directory with no index.html instead of listing it:
//
//	http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
//		name := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
//		if name == "" {
//			name = "."
//		}
//		if !fs.ValidPath(name) {
//			http.NotFound(response, request)
//			return
//		}
//		if _, err := fs.ReadDir(staticAssets, name); err == nil {
//			if _, err := fs.Stat(staticAssets, path.Join(name, "index.html")); err != nil {
//				http.NotFound(response, request)
//				return
//			}
//		}
//		http.FileServerFS(staticAssets).ServeHTTP(response, request)
//	})
//
// The request path has the prefix stripped, so the assets directory
// itself is "". The name is cleaned as http.FileServerFS cleans it, with a
// leading slash, so the two never read different paths, and a name the
// file system rejects is a 404 rather than the file server's choice.
func staticAssetsHandler(file *File) ast.Expr {
	src := fmt.Sprintf(`%[1]s.HandlerFunc(func(response %[1]s.ResponseWriter, request *%[1]s.Request) {
	%[5]s := %[6]s.TrimPrefix(%[2]s.Clean("/"+request.URL.Path), "/")
	if %[5]s == "" {
		%[5]s = "."
	}
	if !%[3]s.ValidPath(%[5]s) {
		%[1]s.NotFound(response, request)
		return
	}
	if _, err := %[3]s.ReadDir(%[4]s, %[5]s); err == nil {
		if _, err := %[3]s.Stat(%[4]s, %[2]s.Join(%[5]s, "index.html")); err != nil {
			%[1]s.NotFound(response, request)
			return
		}
	}
	%[1]s.FileServerFS(%[4]s).ServeHTTP(response, request)
})`, file.Import("http", "net/http"), file.Import("path", "path"), file.Import("fs", "io/fs"), staticAssetsIdent, staticAssetsNameIdent, file.Import("strings", "strings"))
	handler, err := parser.ParseExpr(src)
	if err != nil && file.err == nil {
		file.err = fmt.Errorf("generate: static assets handler: %w", err)
	}
	return handler
}

// staticAssetsPathMethods declare the route builder and path methods for
// the static assets route, named as a call-less GET route with its path:
//
//	func (routes TemplateRouteBuilder) ReadStatic() TemplateRoute {
//		return TemplateRoute{method: http.MethodGet, path: path.Join(cmp.Or(routes.pathsPrefix, "/"), "static") + "/"}
//	}
//
//	func (routePaths TemplateRoutePaths) ReadStatic() string {
//		return routePaths.routes.ReadStatic().Path()
//	}
func staticAssetsPathMethods(file *File, config RoutesFileConfiguration) []ast.Decl {
	prefix := StaticAssetsPrefix(config.StaticAssetsPath)
	joined := &ast.BinaryExpr{
		X:  astgen.Call(file, "path", "path", "Join", pathPrefixOrRoot(file), astgen.String(strings.Trim(prefix, "/"))),
		Op: token.ADD,
		Y:  astgen.String("/"),
	}
	route := &ast.CompositeLit{
		Type: ast.NewIdent(routeTypeName(config)),
		Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent(routeMethodFieldName), Value: astgen.HTTPMethod(file, http.MethodGet)},
			&ast.KeyValueExpr{Key: ast.NewIdent(routePathFieldName), Value: joined},
		},
	}
	builder := routeBuilderMethod(config, muxt.StaticAssetsPathIdentifier(prefix), nil, results(ast.NewIdent(routeTypeName(config))), returnExprs(route))
	return []ast.Decl{builder, routePathWrapper(config, builder)}
}
