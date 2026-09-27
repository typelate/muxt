package source_test

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
)

func named(pkg *types.Package, name string, underlying types.Type) types.Type {
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), underlying, nil)
}

func TestTypeFormat(t *testing.T) {
	out := types.NewPackage("example.com/server", "server")
	model := types.NewPackage("example.com/lib/model", "model")
	qualify := func(name, path string) string {
		if path == out.Path() {
			return ""
		}
		return name + "Alias"
	}
	errorType := types.Universe.Lookup("error").Type()
	for _, tt := range []struct {
		name string
		tp   types.Type
		want string
	}{
		{name: "a basic type", tp: types.Typ[types.Int], want: "int"},
		{name: "a type in the output package", tp: named(out, "T", types.NewStruct(nil, nil)), want: "T"},
		{name: "a type in another package", tp: named(model, "User", types.NewStruct(nil, nil)), want: "modelAlias.User"},
		{name: "a pointer", tp: types.NewPointer(named(model, "User", types.NewStruct(nil, nil))), want: "*modelAlias.User"},
		{name: "a slice", tp: types.NewSlice(types.Typ[types.String]), want: "[]string"},
		{name: "the empty struct", tp: types.NewStruct(nil, nil), want: "struct{}"},
		{
			name: "a signature",
			tp: types.NewSignatureType(nil, nil, nil,
				types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.Int])),
				types.NewTuple(types.NewVar(token.NoPos, nil, "", errorType)),
				false),
			want: "func(int) error",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, source.NewType(tt.tp).Format(qualify))
		})
	}
}

func TestTypeIsString(t *testing.T) {
	pkg := types.NewPackage("example.com/server", "server")
	require.True(t, source.NewType(types.Typ[types.String]).IsString())
	require.True(t, source.NewType(named(pkg, "ID", types.Typ[types.String])).IsString(), "a named string type")
	require.False(t, source.NewType(types.Typ[types.Int]).IsString())
	require.False(t, source.NewType(types.NewStruct(nil, nil)).IsString())
}

func TestTypeBasic(t *testing.T) {
	pkg := types.NewPackage("example.com/server", "server")
	kind, ok := source.NewType(types.Typ[types.Int8]).Basic()
	require.True(t, ok)
	require.Equal(t, types.Int8, kind)

	kind, ok = source.NewType(named(pkg, "Count", types.Typ[types.Uint])).Basic()
	require.True(t, ok, "a named basic type")
	require.Equal(t, types.Uint, kind)

	_, ok = source.NewType(types.NewStruct(nil, nil)).Basic()
	require.False(t, ok)
}

func TestTypeIdentical(t *testing.T) {
	pkg := types.NewPackage("example.com/server", "server")
	id := named(pkg, "ID", types.Typ[types.String])
	require.True(t, source.NewType(id).Identical(source.NewType(id)))
	require.True(t, source.NewType(types.Typ[types.String]).Identical(source.NewType(types.Typ[types.String])))
	require.False(t, source.NewType(id).Identical(source.NewType(types.Typ[types.String])), "a named type is not its underlying type")
	require.False(t, source.NewType(types.Typ[types.Int]).Identical(source.NewType(types.Typ[types.String])))
}

func TestTypeIsZero(t *testing.T) {
	require.True(t, source.Type{}.IsZero())
	require.False(t, source.NewType(types.Typ[types.Int]).IsZero())
}
