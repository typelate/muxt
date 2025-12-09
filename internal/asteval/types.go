package asteval

import "go/types"

func NamedEmptyStruct(ident string, pkg *types.Package) *types.Named {
	return types.NewNamed(types.NewTypeName(0, pkg, ident, nil), types.NewStruct(nil, nil), nil)
}
