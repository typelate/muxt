// The helpers here are private copies of the analysis package's loaders:
// analysis imports generate, so generate cannot import them back.
package generate

import (
	"fmt"
	"go/types"
	"html/template"
	"path/filepath"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"
)

func packageAtFilepath(list []*packages.Package, dir string) (*packages.Package, bool) {
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

func packageWithPath(list []*packages.Package, path string) (*packages.Package, bool) {
	for _, pkg := range list {
		if pkg.PkgPath == path {
			return pkg, true
		}
	}
	return nil, false
}

func findType(pl []*packages.Package, packagePath, ident string) (*types.Named, error) {
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

func namedEmptyStruct(ident string, pkg *types.Package) *types.Named {
	return types.NewNamed(types.NewTypeName(0, pkg, ident, nil), types.NewStruct(nil, nil), nil)
}

// loadTemplates evaluates the package-level template variable through
// check.LoadTemplates and returns the html/template value; muxt
// introspects template names and trees without executing, so a
// text/template set works through an html/template value carrying the
// same trees.
func loadTemplates(templatesVariable string, pkg *packages.Package) (*template.Template, error) {
	lt, err := check.LoadTemplates(pkg, templatesVariable)
	if err != nil {
		return nil, err
	}
	if ts, ok := lt.HTML(); ok {
		return ts, nil
	}
	textTemplates, ok := lt.Text()
	if !ok {
		return nil, fmt.Errorf("variable %s is not a template", templatesVariable)
	}
	ts := template.New(textTemplates.Name())
	for _, t := range textTemplates.Templates() {
		if t.Tree == nil {
			continue
		}
		if _, err := ts.AddParseTree(t.Name(), t.Tree); err != nil {
			return nil, fmt.Errorf("adopting text/template %q: %w", t.Name(), err)
		}
	}
	return ts, nil
}
