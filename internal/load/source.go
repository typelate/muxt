package load

import (
	"cmp"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/source"
)

// Package reads the package at dir among pl, with the templates variables
// named, in order, into a source.Package. It is where a go/packages result
// stops: past it, a run holds go/types values and template sets and
// nothing that needs the go command.
//
// It fails when no loaded package is at dir, or at the first variable that
// does not evaluate to a template set.
func Package(dir string, pl []*packages.Package, variables []string) (source.Package, error) {
	pkg, ok := PackageInDirectory(pl, dir)
	if !ok {
		return source.Package{}, NoPackageError(dir, pl)
	}
	result := source.Package{
		Fset:    pkg.Fset,
		Types:   pkg.Types,
		Imports: imports(pl),
	}
	for _, name := range variables {
		variable, err := Variable(pkg, name)
		if err != nil {
			return source.Package{}, err
		}
		result.Variables = append(result.Variables, variable)
	}
	return result, nil
}

// Variable evaluates the templates variable name in pkg: its template set,
// the functions its templates may call, where each template was defined,
// and the ExecuteTemplate calls made on it.
func Variable(pkg *packages.Package, name string) (source.Variable, error) {
	lt, ts, err := HTMLTemplates(name, pkg)
	if err != nil {
		return source.Variable{}, err
	}
	variable := source.Variable{
		Name:        name,
		Set:         ts,
		Functions:   lt.Functions(),
		Funcs:       lt.CollectedFunctions(),
		Definitions: make(map[string]source.Definition),
	}
	for _, t := range ts.Templates() {
		d, ok := lt.FindDefinition(t.Name())
		if !ok {
			continue
		}
		variable.Definitions[t.Name()] = source.Definition{
			Name:         d.Name,
			Define:       source.Span{Position: d.Define.Position, Length: d.Define.Length},
			End:          source.Span{Position: d.End.Position, Length: d.End.Length},
			TemplateName: source.Span{Position: d.TemplateName.Position, Length: d.TemplateName.Length},
			Tree:         d.Tree,
		}
	}
	for call := range lt.ExecuteTemplateCalls() {
		variable.Calls = append(variable.Calls, source.Call{
			Position: pkg.Fset.Position(call.Call.Pos()),
			Template: call.TemplateName,
			Data:     call.DataType,
		})
	}
	return variable, nil
}

// Receiver finds the receiver type named ident in the package at dir among
// pl, or in the package with import path packagePath when it is set.
func Receiver(dir string, pl []*packages.Package, packagePath, ident string) (*types.Named, error) {
	pkg, ok := PackageInDirectory(pl, dir)
	if !ok {
		return nil, NoPackageError(dir, pl)
	}
	return FindType(pl, cmp.Or(packagePath, pkg.PkgPath), ident)
}

// imports indexes, by import path, every package in pl and every package
// they import. A path loaded more than once -- a package and its test
// variant -- keeps the first in pl.
func imports(pl []*packages.Package) map[string]*types.Package {
	index := make(map[string]*types.Package)
	var queue []*types.Package
	for _, pkg := range pl {
		if pkg.Types == nil {
			continue
		}
		if _, seen := index[pkg.Types.Path()]; !seen {
			index[pkg.Types.Path()] = pkg.Types
			queue = append(queue, pkg.Types)
		}
	}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		for _, imported := range pkg.Imports() {
			if _, seen := index[imported.Path()]; seen {
				continue
			}
			index[imported.Path()] = imported
			queue = append(queue, imported)
		}
	}
	return index
}
