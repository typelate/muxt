// Package templateset holds a templates variable as type checking reads
// it: the template set, where its trees and definitions are, the functions
// a template may call, and the ExecuteTemplate calls made on the variable.
//
// internal/load builds a Variable from a loaded package. It is plain data,
// so a test can build one from templates parsed in memory.
package templateset

import (
	"github.com/typelate/check"

	"github.com/typelate/muxt/internal/muxt"
)

// Variable is one templates variable, ready for check.Execute.
type Variable struct {
	muxt.Templates

	Trees       check.TreeFinder
	Definitions check.DefinitionFinder

	// Functions are the builtins and the functions registered with Funcs.
	Functions check.Functions

	// Calls are the variable's ExecuteTemplate calls, in file order.
	Calls []check.ExecuteTemplateCall
}

// Global wires a check.Global for type checking the variable's templates
// in pkg.
func (v Variable) Global(pkg muxt.Package) *check.Global {
	global := check.NewGlobal(pkg.Types, pkg.Fset, v.Trees, v.Functions)
	global.Definitions = v.Definitions
	return global
}
