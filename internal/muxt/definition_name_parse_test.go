package muxt_test

import (
	"testing"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

func TestParseDefinitionName(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		ok                         bool
		method, host, path, status string
		call                       string // astgen.Format of Call; "" when nil
	}{
		{name: "GET  /x/{id}   M( ctx , id )", ok: true, method: "GET", path: "/x/{id}", call: "M(ctx, id)"},
		{name: "POST example.com/x 201 M(ctx)", ok: true, method: "POST", host: "example.com", path: "/x", status: "201", call: "M(ctx)"},
		{name: "/x", ok: true, path: "/x"},
		{name: "FOO /x M(ctx, \"s\")", ok: true, method: "FOO", path: "/x", call: `M(ctx, "s")`},
		{name: "partial"},
		{name: "with  spaces"},
		{name: "GET /x M(ctx,"},
		{name: "GET /x M"},
		{name: "GET /x a.M(ctx)"},
	} {
		got, ok := muxt.ParseDefinitionName(tt.name)
		if ok != tt.ok {
			t.Errorf("ParseDefinitionName(%q) ok = %t, want %t", tt.name, ok, tt.ok)
			continue
		}
		var call string
		if got.Call != nil {
			call = astgen.Format(got.Call)
		}
		if got.Method != tt.method || got.Host != tt.host || got.Path != tt.path || got.Status != tt.status || call != tt.call {
			t.Errorf("ParseDefinitionName(%q) = %q %q %q %q %q, want %q %q %q %q %q", tt.name, got.Method, got.Host, got.Path, got.Status, call, tt.method, tt.host, tt.path, tt.status, tt.call)
		}
	}
}
