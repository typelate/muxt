package asteval

import (
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

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
