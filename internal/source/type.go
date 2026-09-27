package source

import "go/types"

// Type is a Go type as what runs after resolution reads it: it formats to
// source text against a file's imports and answers the questions generation
// asks, so those packages need not import go/types.
type Type struct {
	tp types.Type
}

func NewType(tp types.Type) Type { return Type{tp: tp} }

// Format writes the type as it is spelled in a file that refers to package
// path by what qualify returns for it, or unqualified when qualify returns
// "".
func (t Type) Format(qualify func(pkgName, pkgPath string) string) string {
	return types.TypeString(t.tp, func(pkg *types.Package) string {
		return qualify(pkg.Name(), pkg.Path())
	})
}

func (t Type) Identical(u Type) bool { return types.Identical(t.tp, u.tp) }

// IsString reports whether the underlying type is string.
func (t Type) IsString() bool {
	kind, ok := t.Basic()
	return ok && kind == types.String
}

// Basic returns the kind of the underlying basic type, if it has one.
func (t Type) Basic() (types.BasicKind, bool) {
	basic, ok := t.tp.Underlying().(*types.Basic)
	if !ok {
		return types.Invalid, false
	}
	return basic.Kind(), true
}
