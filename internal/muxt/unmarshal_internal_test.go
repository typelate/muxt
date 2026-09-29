package muxt

import (
	"go/types"
	"testing"
)

type textUnmarshalerChecker struct {
	scopeChecker
	unmarshalers []types.Type
}

func (c textUnmarshalerChecker) TextUnmarshaler(tp types.Type) bool {
	for _, candidate := range c.unmarshalers {
		if types.Identical(candidate, tp) {
			return true
		}
	}
	return false
}

func TestUnmarshalMethodFor(t *testing.T) {
	pkg := checkSource(t, `package p

type ID int
type Plain int
type Alias = string

var (
	Slice []string
	Map map[string]string
	Struct struct{}
	Pointer *int
	Byte byte
	Rune rune
	Complex complex128
	Uintptr uintptr
)
`)
	lookup := func(name string) types.Type { return pkg.Scope().Lookup(name).Type() }
	checker := textUnmarshalerChecker{unmarshalers: []types.Type{lookup("ID")}}
	for _, tt := range []struct {
		name string
		tp   types.Type
		want UnmarshalMethod
	}{
		{name: "string", tp: types.Typ[types.String], want: UnmarshalString},
		{name: "bool", tp: types.Typ[types.Bool], want: UnmarshalBool},
		{name: "int", tp: types.Typ[types.Int], want: UnmarshalInt},
		{name: "int8", tp: types.Typ[types.Int8], want: UnmarshalInt8},
		{name: "int16", tp: types.Typ[types.Int16], want: UnmarshalInt16},
		{name: "int32", tp: types.Typ[types.Int32], want: UnmarshalInt32},
		{name: "int64", tp: types.Typ[types.Int64], want: UnmarshalInt64},
		{name: "uint", tp: types.Typ[types.Uint], want: UnmarshalUint},
		{name: "uint8", tp: types.Typ[types.Uint8], want: UnmarshalUint8},
		{name: "uint16", tp: types.Typ[types.Uint16], want: UnmarshalUint16},
		{name: "uint32", tp: types.Typ[types.Uint32], want: UnmarshalUint32},
		{name: "uint64", tp: types.Typ[types.Uint64], want: UnmarshalUint64},
		{name: "float32", tp: types.Typ[types.Float32], want: UnmarshalFloat32},
		{name: "float64", tp: types.Typ[types.Float64], want: UnmarshalFloat64},
		{name: "named type the checker says unmarshals text", tp: lookup("ID"), want: UnmarshalTextUnmarshaler},
		{name: "named type the checker does not know", tp: lookup("Plain"), want: UnmarshalUnsupported},
		{name: "byte alias is matched by name and not supported", tp: lookup("Byte"), want: UnmarshalUnsupported},
		{name: "rune alias is matched by name and not supported", tp: lookup("Rune"), want: UnmarshalUnsupported},
		{name: "complex", tp: lookup("Complex"), want: UnmarshalUnsupported},
		{name: "uintptr", tp: lookup("Uintptr"), want: UnmarshalUnsupported},
		{name: "slice", tp: lookup("Slice"), want: UnmarshalUnsupported},
		{name: "map", tp: lookup("Map"), want: UnmarshalUnsupported},
		{name: "struct", tp: lookup("Struct"), want: UnmarshalUnsupported},
		{name: "pointer", tp: lookup("Pointer"), want: UnmarshalUnsupported},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := unmarshalMethodFor(checker, tt.tp); got != tt.want {
				t.Errorf("unmarshalMethodFor(%s) = %d, want %d", tt.tp, got, tt.want)
			}
		})
	}
}
