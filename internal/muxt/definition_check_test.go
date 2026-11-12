package muxt_test

import (
	"errors"
	"testing"

	"github.com/typelate/muxt/internal/muxt"
)

// TestCheckDefinitionName states that a template name is accepted when muxt
// reads it as a route, rejected as not a route when it has no route shape, and
// rejected with the error muxt gives a route name that is malformed.
func TestCheckDefinitionName(t *testing.T) {
	for _, tt := range []struct {
		name    string
		wantErr string
		notName bool
	}{
		{name: "GET /foo Index()"},
		{name: "POST /foo/{id} 201 Update(ctx, id)"},
		{name: "header", wantErr: `"header" is not a route template name`, notName: true},
		{name: "GET foo Index(", wantErr: `"GET foo Index(" is not a route template name`, notName: true},
		{name: "FOO /foo", wantErr: "FOO method not allowed; allowed methods: GET, POST, PUT, PATCH, and DELETE"},
		{name: "GET /foo Index(", wantErr: "failed to parse handler expression: template_name.go:1:7: expected ')', found 'EOF'"},
	} {
		err := muxt.CheckDefinitionName(tt.name)
		if tt.wantErr == "" {
			if err != nil {
				t.Errorf("CheckDefinitionName(%q) = %v, want nil", tt.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("CheckDefinitionName(%q) = nil, want error %q", tt.name, tt.wantErr)
			continue
		}
		if err.Error() != tt.wantErr {
			t.Errorf("CheckDefinitionName(%q) = %q, want %q", tt.name, err.Error(), tt.wantErr)
		}
		var notRoute *muxt.NotRouteNameError
		if got := errors.As(err, &notRoute); got != tt.notName {
			t.Errorf("CheckDefinitionName(%q) is a *NotRouteNameError = %t, want %t", tt.name, got, tt.notName)
		}
	}
}
