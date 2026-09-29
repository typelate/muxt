package generate

import (
	"testing"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

func TestRequestArgumentSource(t *testing.T) {
	for _, tt := range []struct {
		name     string
		argument muxt.Argument
		want     string
		wantErr  string
	}{
		{name: "body", argument: muxt.Argument{Type: muxt.ArgumentTypeRequestBody, Identifier: "body"}, want: "request.Body"},
		{name: "last event id", argument: muxt.Argument{Type: muxt.ArgumentTypeLastEventID, Identifier: "lastEventID"}, want: `request.Header.Get("Last-Event-Id")`},
		{name: "path value", argument: muxt.Argument{Type: muxt.ArgumentTypeRequestPathValue, Identifier: "id"}, want: `request.PathValue("id")`},
		{name: "unsupported", argument: muxt.Argument{Type: muxt.ArgumentTypeRequestContext, Identifier: "ctx"}, wantErr: "no request source for argument ctx"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requestArgumentSource(tt.argument)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("requestArgumentSource(%s) error = %v, want %q", tt.name, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s := astgen.Format(got); s != tt.want {
				t.Errorf("requestArgumentSource(%s) = %q, want %q", tt.name, s, tt.want)
			}
		})
	}
}
