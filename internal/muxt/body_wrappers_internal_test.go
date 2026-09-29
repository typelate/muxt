package muxt

import "testing"

func TestScanBodyBindings(t *testing.T) {
	for _, tt := range []struct {
		name string
		call string
		want bodyBindings
	}{
		{name: "no arguments", call: `F()`},
		{name: "body identifier", call: `F(body)`, want: bodyBindings{reads: 1}},
		{name: "form identifier", call: `F(form)`, want: bodyBindings{hasForm: true}},
		{name: "multipart identifier", call: `F(multipart)`, want: bodyBindings{hasMultipart: true}},
		{name: "unrelated identifiers", call: `F(ctx, request, 5)`},
		{name: "unmarshalJSON wrapper", call: `F(unmarshalJSON(body))`, want: bodyBindings{reads: 1}},
		{name: "unmarshalForm wrapper is the form binding", call: `F(unmarshalForm(body))`, want: bodyBindings{hasForm: true}},
		{name: "two reads", call: `F(body, unmarshalJSON(body))`, want: bodyBindings{reads: 2}},
		{name: "nested call", call: `F(G(body, form))`, want: bodyBindings{reads: 1, hasForm: true}},
		{name: "nested multipart", call: `F(G(H(multipart)))`, want: bodyBindings{hasMultipart: true}},
		{name: "nested and direct", call: `F(form, G(multipart, body))`, want: bodyBindings{reads: 1, hasForm: true, hasMultipart: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := scanBodyBindings(mustParseCall(t, tt.call)); got != tt.want {
				t.Errorf("scanBodyBindings(%s) = %+v, want %+v", tt.call, got, tt.want)
			}
		})
	}
}
