package muxt

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestArgumentErrorCanBeNil states which error results of a nested call can
// be nil: a pointer or interface can, a struct cannot.
func TestArgumentErrorCanBeNil(t *testing.T) {
	pkg := types.NewPackage("example.com/server", "server")
	structError := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "ValueError", nil), types.NewStruct(nil, nil), nil)
	pointerError := types.NewPointer(types.NewNamed(types.NewTypeName(token.NoPos, pkg, "LookupError", nil), types.NewStruct(nil, nil), nil))
	errorType := types.Universe.Lookup("error").Type()
	signature := func(results ...types.Type) *types.Signature {
		vars := make([]*types.Var, 0, len(results))
		for _, result := range results {
			vars = append(vars, types.NewParam(token.NoPos, pkg, "", result))
		}
		return types.NewSignatureType(nil, nil, nil, nil, types.NewTuple(vars...), false)
	}
	for _, tt := range []struct {
		name string
		sig  *types.Signature
		want bool
	}{
		{name: "a leaf argument", want: true},
		{name: "one result", sig: signature(types.Typ[types.Int]), want: true},
		{name: "the error interface", sig: signature(types.Typ[types.Int], errorType), want: true},
		{name: "a pointer error", sig: signature(types.Typ[types.Int], pointerError), want: true},
		{name: "a struct error", sig: signature(types.Typ[types.Int], structError), want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Argument{sig: tt.sig}.ErrorCanBeNil())
		})
	}
}
