package muxt

import (
	"go/token"
	"go/types"
	"html/template"
)

// Package is the Go package routes are read from, as go/types sees it.
//
// It is everything route resolution needs from a loaded package and
// nothing about how the package was loaded. Loading takes the go
// command (see internal/load); building a Package by hand from
// type-checked source takes microseconds, which is what lets resolution
// and generation be tested without a module on disk.
type Package struct {
	// Fset positions the objects in Types and in the packages Lookup
	// returns.
	Fset *token.FileSet

	// Types is the package that declares the templates variables. Its
	// package-scope functions may be called from a template name.
	Types *types.Package

	// Lookup finds a package loaded alongside Types by import path:
	// the standard library packages a route argument's type comes from
	// (net/http, context, encoding, ...). It may be nil, in which case
	// only Types and its transitive imports are searched.
	Lookup func(path string) (*types.Package, bool)
}

// Import finds the package with path: through Lookup when it is set,
// otherwise among Types and the packages it imports, transitively.
func (pkg Package) Import(path string) (*types.Package, bool) {
	if pkg.Lookup != nil {
		return pkg.Lookup(path)
	}
	if pkg.Types == nil {
		return nil, false
	}
	if pkg.Types.Path() == path {
		return pkg.Types, true
	}
	return SearchImports(pkg.Types, path)
}

// SearchImports looks for the package with path among the imports of
// pt, breadth first at each level: the direct imports before any of
// theirs.
func SearchImports(pt *types.Package, path string) (*types.Package, bool) {
	for _, pkg := range pt.Imports() {
		if pkg.Path() == path {
			return pkg, true
		}
	}
	for _, pkg := range pt.Imports() {
		if p, ok := SearchImports(pkg, path); ok {
			return p, true
		}
	}
	return nil, false
}

// Templates is one templates variable: the template set it holds and
// what is known about where its templates were written.
type Templates struct {
	// Variable names the package-level variable holding the set.
	Variable string

	// Set is the template set. A text/template variable is carried as
	// an html/template value with the same trees: muxt reads names and
	// trees and never executes the set.
	Set *template.Template

	// Functions are the functions registered with Funcs calls in the
	// variable's construction chain, without the builtins.
	Functions map[string]*types.Signature

	// NamePosition reports where a template's name begins in the
	// source that defines it: the first byte inside the name's quotes.
	// It may be nil when source locations are unknown.
	NamePosition func(templateName string) (token.Position, bool)

	// Err is why the variable could not be loaded. When it is set the
	// other fields are unset, and a caller reports Err at the point it
	// would have read the set, so errors keep the order they would have
	// had if each variable were loaded on demand.
	Err error
}

// Source is what a run reads routes from: the package, the receiver type
// named on the command line, and each templates variable in the order it
// was named.
type Source struct {
	Package Package

	// Receiver is the type --use-receiver-type named, or nil when no
	// type was named and handler methods are inferred from the
	// templates.
	Receiver *types.Named

	Templates []Templates
}
