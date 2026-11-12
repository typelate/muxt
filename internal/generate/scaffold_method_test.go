package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMethodStub states the formatted method a receiver needs for a route
// that calls a method it does not define: a body that returns nil for each
// result, and none for a method with no results.
func TestMethodStub(t *testing.T) {
	for _, tt := range []struct {
		name string
		spec MethodSpec
		want string
	}{
		{
			name: "value receiver",
			spec: MethodSpec{Receiver: "T", Name: "Index", Params: []MethodParam{{Name: "ctx", Type: "context.Context"}, {Name: "id", Type: "string"}}, Results: []string{"any"}},
			want: "func (T) Index(ctx context.Context, id string) any {\n\treturn nil\n}\n",
		},
		{
			name: "pointer receiver",
			spec: MethodSpec{Receiver: "T", Pointer: true, Name: "Index", Results: []string{"any"}},
			want: "func (*T) Index() any {\n\treturn nil\n}\n",
		},
		{
			name: "no results",
			spec: MethodSpec{Receiver: "T", Name: "Events", Params: []MethodParam{{Name: "send", Type: "func(any) error"}}},
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
