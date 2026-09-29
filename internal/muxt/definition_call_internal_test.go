package muxt

import "testing"

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
			if got := def.callWriteHeader(); got != tt.want {
				t.Errorf("callWriteHeader(%s) = %t, want %t", tt.call, got, tt.want)
			}
		})
	}
	if !(Definition{}).callWriteHeader() {
		t.Error("callWriteHeader() of a definition without a call = false, want true")
	}
}
