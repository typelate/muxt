package generate

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMethodStub states the formatted method a receiver needs for a route
// that calls a method it does not define: a body that returns nil for each
// result, and none for a method with no results.
func TestMethodStub(t *testing.T) {
	anyType := types.Universe.Lookup("any").Type()
	contextPkg := types.NewPackage("context", "context")
	contextName := types.NewTypeName(token.NoPos, contextPkg, "Context", nil)
	contextType := types.NewNamed(contextName, types.NewInterfaceType(nil, nil), nil)
	errorType := types.Universe.Lookup("error").Type()
	sendType := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "", anyType)),
		types.NewTuple(types.NewVar(token.NoPos, nil, "", errorType)), false)
	for _, tt := range []struct {
		name string
		spec MethodSpec
		want string
	}{
		{
			name: "value receiver",
			spec: MethodSpec{Receiver: "T", Name: "Index", Params: []MethodParam{{Name: "ctx", Type: contextType}, {Name: "id", Type: types.Typ[types.String]}}, Results: []types.Type{anyType}},
			want: "func (T) Index(ctx context.Context, id string) any {\n\treturn nil\n}\n",
		},
		{
			name: "pointer receiver",
			spec: MethodSpec{Receiver: "T", Pointer: true, Name: "Index", Results: []types.Type{anyType}},
			want: "func (*T) Index() any {\n\treturn nil\n}\n",
		},
		{
			name: "no results",
			spec: MethodSpec{Receiver: "T", Name: "Events", Params: []MethodParam{{Name: "send", Type: sendType}}},
			want: "func (T) Events(send func(any) error) {\n}\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MethodStub(tt.spec)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got, "MethodStub(%+v)", tt.spec)
		})
	}
}
