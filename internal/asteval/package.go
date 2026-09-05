package asteval

import (
	"fmt"
	"go/token"
	"go/types"
	"html/template"
	"path/filepath"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"
)

func LoadPackages(wd string, morePatterns ...string) (*token.FileSet, []*packages.Package, error) {
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
	}, patterns...)
	if err != nil {
		return nil, nil, err
	}
	return fileSet, pl, err
}

func PackageAtFilepath(list []*packages.Package, dir string) (*packages.Package, bool) {
	d := dir
	if filepath.Ext(d) == ".go" {
		d = filepath.Dir(dir)
	}
	for _, pkg := range list {
		if len(pkg.GoFiles) > 0 && filepath.Dir(pkg.GoFiles[0]) == d {
			return pkg, true
		}
	}
	return nil, false
}

func PackageWithPath(list []*packages.Package, path string) (*packages.Package, bool) {
	for _, pkg := range list {
		if pkg.PkgPath == path {
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

func LoadTemplates(wd, templatesVariable string, pl []*packages.Package) (*LoadedTemplates, error) {
	pkg, ok := PackageAtFilepath(pl, wd)
	if !ok {
		return nil, fmt.Errorf("package not found at %s", wd)
	}

	lt, ts, err := loadHTMLTemplates(templatesVariable, pkg)
	if err != nil {
		return nil, err
	}

	global := check.NewGlobal(pkg.Types, pkg.Fset, lt, lt.Functions())
	global.Definitions = lt
	return &LoadedTemplates{Package: pkg, Templates: lt, Global: global, HTML: ts}, nil
}

// Templates evaluates the package-level template variable through
// check.LoadTemplates and returns the html/template value together with
// the functions collected from Funcs calls in its construction chain.
func Templates(templatesVariable string, pkg *packages.Package) (*template.Template, check.Functions, error) {
	lt, ts, err := loadHTMLTemplates(templatesVariable, pkg)
	if err != nil {
		return nil, nil, err
	}
	return ts, lt.CollectedFunctions(), nil
}

func loadHTMLTemplates(templatesVariable string, pkg *packages.Package) (*check.Templates, *template.Template, error) {
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
