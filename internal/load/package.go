package load

import (
	"fmt"
	"go/token"
	"go/types"
	"html/template"
	"path/filepath"
	"strings"

	"github.com/typelate/check"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/astgen"
)

func Packages(wd string, morePatterns ...string) (*token.FileSet, []*packages.Package, error) {
	return PackagesWithEnv(wd, nil, morePatterns...)
}

// PackagesWithEnv is Packages with the environment the go command
// runs in. A nil env is the process's own.
func PackagesWithEnv(wd string, env []string, morePatterns ...string) (*token.FileSet, []*packages.Package, error) {
	patterns := []string{
		wd, "encoding", "fmt", "net/http",
	}
	for _, pat := range morePatterns {
		if pat != "" {
			patterns = append(patterns, pat)
		}
	}
	fileSet := token.NewFileSet()
	pl, err := packages.Load(&packages.Config{
		Fset: fileSet,
		Mode: packages.NeedModule | packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles | packages.NeedImports,
		Dir:  wd,
		Env:  env,
	}, patterns...)
	if err != nil {
		return nil, nil, loadFailedError(wd, err)
	}
	return fileSet, pl, err
}

// PackagesWithTests is PackagesWithEnv, loading each package with its
// in-package test files too.
//
// With tests, go list reports a package twice: once as it is written and
// once compiled with its test files. The second holds a test's
// ExecuteTemplate calls, so the test variants come first, ahead of the
// packages as written, and a package picked by directory is the one with
// its tests. Unlike PackagesWithEnv, a load failure is returned as the
// loader reported it.
func PackagesWithTests(wd string, env []string) ([]*packages.Package, error) {
	pl, err := packages.Load(&packages.Config{
		Fset:  token.NewFileSet(),
		Tests: true,
		Mode: packages.NeedModule | packages.NeedTypesInfo | packages.NeedName |
			packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedEmbedPatterns | packages.NeedEmbedFiles | packages.NeedImports,
		Dir: wd,
		Env: env,
	}, wd, "encoding", "fmt", "net/http")
	if err != nil {
		return nil, err
	}
	ordered := make([]*packages.Package, 0, len(pl))
	for _, pkg := range pl {
		if strings.HasSuffix(pkg.ID, ".test]") {
			ordered = append(ordered, pkg)
		}
	}
	for _, pkg := range pl {
		if !strings.HasSuffix(pkg.ID, ".test]") {
			ordered = append(ordered, pkg)
		}
	}
	return ordered, nil
}

// ParseErrors returns the syntax errors the loader recovered from.
//
// The loader carries on with a partial AST, so a caller still gets
// answers -- they are just answers about source that is missing whatever
// the parser could not read. Reporting that is the caller's job: go build
// is what gates on a package compiling, and muxt's own checks are worth
// running on broken source, so this reads as a warning rather than a
// refusal.
//
// Only syntax errors. A type error is how a package looks before muxt
// generate has written its handlers -- main.go calling a TemplateRoutes
// that does not exist yet -- so warning about those would fire on the
// ordinary first run.
func ParseErrors(pl []*packages.Package) []packages.Error {
	var found []packages.Error
	seen := make(map[string]struct{})
	for _, pkg := range pl {
		for _, e := range pkg.Errors {
			if e.Kind != packages.ParseError {
				continue
			}
			// One broken file is reported once per package that reads it.
			if _, dup := seen[e.Error()]; dup {
				continue
			}
			seen[e.Error()] = struct{}{}
			found = append(found, e)
		}
	}
	return found
}

// PackageInDirectory returns the package whose files are in dir, which is
// always a directory, even one whose name ends in .go.
func PackageInDirectory(list []*packages.Package, dir string) (*packages.Package, bool) {
	for _, pkg := range list {
		if len(pkg.GoFiles) > 0 && filepath.Dir(pkg.GoFiles[0]) == dir {
			return pkg, true
		}
	}
	return nil, false
}

// LoadedTemplates bundles a package's loaded template variable with the
// analysis wiring built from it.
type LoadedTemplates struct {
	Package   *packages.Package
	Templates *check.Templates
	Global    *check.Global
	HTML      *template.Template
}

func Templates(wd, templatesVariable string, pl []*packages.Package) (*LoadedTemplates, error) {
	pkg, ok := PackageInDirectory(pl, wd)
	if !ok {
		return nil, NoPackageError(wd, pl)
	}

	lt, ts, err := HTMLTemplates(templatesVariable, pkg)
	if err != nil {
		return nil, err
	}

	global := check.NewGlobal(pkg.Types, pkg.Fset, lt, lt.Functions())
	global.Definitions = lt
	return &LoadedTemplates{Package: pkg, Templates: lt, Global: global, HTML: ts}, nil
}

// HTMLTemplates evaluates the package-level template variable through
// check.LoadTemplates and returns the loaded handle alongside the
// html/template value; muxt introspects template names and trees without
// executing, so a text/template set works through an html/template value
// carrying the same trees.
func HTMLTemplates(templatesVariable string, pkg *packages.Package) (*check.Templates, *template.Template, error) {
	lt, err := check.LoadTemplates(pkg, templatesVariable)
	if err != nil {
		return nil, nil, err
	}
	if ts, ok := lt.HTML(); ok {
		return lt, ts, nil
	}
	// Muxt introspects template names and trees; it never executes the
	// user's variable, so a text/template set works through an
	// html/template value carrying the same trees.
	textTemplates, ok := lt.Text()
	if !ok {
		return nil, nil, fmt.Errorf("variable %s is not a template", templatesVariable)
	}
	ts := template.New(textTemplates.Name())
	for _, t := range textTemplates.Templates() {
		if t.Tree == nil {
			continue
		}
		if _, err := ts.AddParseTree(t.Name(), t.Tree); err != nil {
			return nil, nil, fmt.Errorf("adopting text/template %q: %w", t.Name(), err)
		}
	}
	return lt, ts, nil
}

func FindType(pl []*packages.Package, packagePath, ident string) (*types.Named, error) {
	notFoundErr := fmt.Errorf("could not find receiver type %s in %s", ident, packagePath)
	for _, pkg := range pl {
		if pkg.PkgPath != packagePath {
			continue
		}
		obj := pkg.Types.Scope().Lookup(ident)
		if obj == nil {
			var typeNames []string
			for _, name := range pkg.Types.Scope().Names() {
				if _, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName); ok {
					typeNames = append(typeNames, name)
				}
			}
			if suggestion, ok := astgen.NearestString(ident, typeNames); ok {
				return nil, fmt.Errorf("could not find receiver type %s in %s; did you mean %s?", ident, packagePath, suggestion)
			}
			return nil, notFoundErr
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			return nil, fmt.Errorf("expected receiver %s to be a named type", ident)
		}
		return named, nil
	}
	return nil, notFoundErr
}
