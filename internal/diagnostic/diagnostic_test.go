package diagnostic_test

import (
	"testing"

	"github.com/typelate/muxt/internal/diagnostic"
)

func TestDiagnosticString(t *testing.T) {
	for _, tt := range []struct {
		name string
		d    diagnostic.Diagnostic
		want string
	}{
		{
			name: "file line column message and fix",
			d: diagnostic.Diagnostic{
				Place:   diagnostic.Place{File: "/m/muxt.json", Line: 3, Column: 7},
				Message: "unknown key",
				Fix:     "remove it",
			},
			want: "/m/muxt.json:3:7: unknown key; remove it",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("Diagnostic.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
