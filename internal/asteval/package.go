package asteval

import (
	"fmt"
	"go/token"
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
		Mode: packages.NeedModule | packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles,
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

func LoadTemplates(wd, templatesVariable string, pl []*packages.Package) (*packages.Package, *check.Global, *template.Template, error) {
	pkg, ok := PackageAtFilepath(pl, wd)
	if !ok {
		return nil, nil, nil, fmt.Errorf("package not found at %s", wd)
	}

	ts, fm, err := Templates(wd, templatesVariable, pkg)
	if err != nil {
		return nil, nil, nil, err
	}

	// Set up the check package for template type analysis
	fns := check.DefaultFunctions(pkg.Types)
	fns = fns.Add(check.Functions(fm))
	global := check.NewGlobal(pkg.Types, pkg.Fset, NewForrest(ts), fns)
	return pkg, global, ts, nil
}
