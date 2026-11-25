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
		{
			name: "warning reads as an error does",
			d: diagnostic.Diagnostic{
				Severity: diagnostic.Warning,
				Place:    diagnostic.Place{File: "/m/muxt.json", Line: 3, Column: 7},
				Message:  "unknown key",
				Fix:      "remove it",
			},
			want: "/m/muxt.json:3:7: unknown key; remove it",
		},
		{
			name: "no fix",
			d: diagnostic.Diagnostic{
				Place:   diagnostic.Place{File: "/m/muxt.json", Line: 3, Column: 7},
				Message: "unknown key",
			},
			want: "/m/muxt.json:3:7: unknown key",
		},
		{
			name: "no file",
			d: diagnostic.Diagnostic{
				Place:   diagnostic.Place{Within: "GET /{id}"},
				Message: "no handler",
				Fix:     "define one",
			},
			want: "GET /{id}: no handler; define one",
		},
		{
			name: "no place",
			d: diagnostic.Diagnostic{
				Message: "no handler",
				Fix:     "define one",
			},
			want: "no handler; define one",
		},
		{
			name: "no line",
			d: diagnostic.Diagnostic{
				Place:   diagnostic.Place{File: "/m/muxt.json"},
				Message: "unknown key",
				Fix:     "remove it",
			},
			want: "/m/muxt.json: unknown key; remove it",
		},
		{
			name: "within the file",
			d: diagnostic.Diagnostic{
				Place:   diagnostic.Place{File: "/m/muxt.json", Within: `packages["a"][0]`},
				Message: "unknown key",
				Fix:     "remove it",
			},
			want: `/m/muxt.json: packages["a"][0]: unknown key; remove it`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("Diagnostic.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
