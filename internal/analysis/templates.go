package analysis

import (
	"github.com/typelate/check"

	"github.com/typelate/muxt/internal/muxt"
)

// Templates is a templates variable with what type checking its
// templates needs: where to find trees and definitions, the functions a
// template may call, and the ExecuteTemplate calls made on the variable.
//
// internal/load builds one from a loaded package. A test builds one from
// a template set parsed in memory.
type Templates struct {
	muxt.Templates

	Trees       check.TreeFinder
	Definitions check.DefinitionFinder

	// Functions are the builtins and the functions registered with Funcs.
	Functions check.Functions

	// Calls are the variable's ExecuteTemplate calls, in file order.
	Calls []check.ExecuteTemplateCall
}

// global wires a check.Global for type checking templates in pkg.
func (t Templates) global(pkg muxt.Package) *check.Global {
	global := check.NewGlobal(pkg.Types, pkg.Fset, t.Trees, t.Functions)
	global.Definitions = t.Definitions
	return global
}
