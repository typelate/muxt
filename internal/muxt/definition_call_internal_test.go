package muxt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCallWriteHeader(t *testing.T) {
	for _, tt := range []struct {
		name string
		call string
		want bool
	}{
		{name: "response is a direct argument", call: `Save(ctx, response)`, want: false},
		{name: "response only", call: `Save(response)`, want: false},
		{name: "no response argument", call: `Save(ctx, request)`, want: true},
		{name: "no arguments", call: `Save()`, want: true},
		{name: "response nested in a call is not direct", call: `Save(Inner(response))`, want: true},
		{name: "a name that contains response", call: `Save(responses)`, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{call: mustParseCall(t, tt.call)}
			assert.Equal(t, tt.want, def.callWriteHeader(), "callWriteHeader(%s)", tt.call)
		})
	}

	t.Run("a definition without a call", func(t *testing.T) {
		assert.True(t, (Definition{}).callWriteHeader(), "callWriteHeader() of a definition without a call")
	})
}
