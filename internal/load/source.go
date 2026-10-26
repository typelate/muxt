package load

import (
	"cmp"
	"go/ast"
	"go/types"
	"iter"

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
	pkg, ok := packageInDirectory(pl, dir)
	if !ok {
		return source.Package{}, NoPackageError(dir, pl)
	}
	result := source.Package{
		Fset:  pkg.Fset,
		Types: pkg.Types,
	}
	for _, name := range variables {
		variable, err := readVariable(pkg, name)
		if err != nil {
			return source.Package{}, err
		}
		result.Variables = append(result.Variables, variable)
	}
	return result, nil
}

// readVariable evaluates the templates variable name in pkg: its template set,
// the functions its templates may call, where each template was defined,
// and the ExecuteTemplate calls made on it.
func readVariable(pkg *packages.Package, name string) (source.Variable, error) {
	lt, ts, err := htmlTemplates(name, pkg)
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
	literal := make(map[*ast.CallExpr]bool)
	for call := range lt.ExecuteTemplateCalls() {
		literal[call.Call] = true
		variable.Calls = append(variable.Calls, source.Call{
			Position: pkg.Fset.Position(call.Call.Pos()),
			Template: call.TemplateName,
			Data:     call.DataType,
		})
	}
	for call := range executeTemplateCalls(pkg, pkg.Types.Scope().Lookup(name)) {
		if !literal[call] {
			variable.NonLiteralCalls = append(variable.NonLiteralCalls, pkg.Fset.Position(call.Pos()))
		}
	}
	return variable, nil
}

// executeTemplateCalls yields every variable.ExecuteTemplate(w, name, data)
// call in pkg, whatever its template name argument is.
func executeTemplateCalls(pkg *packages.Package, variable types.Object) iter.Seq[*ast.CallExpr] {
	return func(yield func(*ast.CallExpr) bool) {
		if variable == nil || pkg.TypesInfo == nil {
			return
		}
		for _, file := range pkg.Syntax {
			keepWalking := true
			ast.Inspect(file, func(node ast.Node) bool {
				if !keepWalking {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) != 3 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "ExecuteTemplate" {
					return true
				}
				receiver, ok := sel.X.(*ast.Ident)
				if !ok || pkg.TypesInfo.Uses[receiver] != variable {
					return true
				}
				keepWalking = yield(call)
				return keepWalking
			})
			if !keepWalking {
				return
			}
		}
	}
}

// Receiver finds the receiver type named ident in the package at dir among
// pl, or in the package with import path packagePath when it is set.
func Receiver(dir string, pl []*packages.Package, packagePath, ident string) (*types.Named, error) {
	pkg, ok := packageInDirectory(pl, dir)
	if !ok {
		return nil, NoPackageError(dir, pl)
	}
	return findType(pl, cmp.Or(packagePath, pkg.PkgPath), ident)
}
