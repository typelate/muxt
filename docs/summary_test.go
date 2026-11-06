package docs_test

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/docs"
)

func TestSummary(t *testing.T) {
	for _, tt := range []struct {
		command   string
		wantOK    bool
		wantStart string
	}{
		{
			command:   "check",
			wantOK:    true,
			wantStart: "Type-checks each template against the data type at its `ExecuteTemplate` call site",
		},
		{
			command:   "version",
			wantOK:    true,
			wantStart: "Prints the binary's module version",
		},
		{
			command: "nope",
		},
	} {
		t.Run(tt.command, func(t *testing.T) {
			got, ok := docs.Summary(tt.command)
			if ok != tt.wantOK {
				t.Fatalf("Summary(%q) ok = %t, want %t", tt.command, ok, tt.wantOK)
			}
			if !tt.wantOK {
				if got != "" {
					t.Errorf("Summary(%q) = %q, want empty", tt.command, got)
				}
				return
			}
			if !strings.HasPrefix(got, tt.wantStart) {
				t.Errorf("Summary(%q) = %q, want prefix %q", tt.command, got, tt.wantStart)
			}
			if strings.Contains(got, "\n") {
				t.Errorf("Summary(%q) = %q, want a single line", tt.command, got)
			}
			if strings.Contains(got, "](") {
				t.Errorf("Summary(%q) = %q, want links flattened to their text", tt.command, got)
			}
		})
	}

	for _, command := range []string{
		"generate", "check", "fmt", "version", "explore-module", "generate-fake-server",
		"list-template-callers", "list-template-calls", "test-template-mutations", "documentation",
	} {
		t.Run("page "+command, func(t *testing.T) {
			got, ok := docs.Summary(command)
			if !ok {
				t.Fatalf("Summary(%q) ok = false, want a page", command)
			}
			if got == "" {
				t.Errorf("Summary(%q) = empty, want a paragraph", command)
			}
			if strings.Contains(got, "\n") {
				t.Errorf("Summary(%q) = %q, want a single line", command, got)
			}
			if strings.HasPrefix(got, "#") || strings.HasPrefix(got, "```") {
				t.Errorf("Summary(%q) = %q, want prose, not a heading or code fence", command, got)
			}
		})
	}
}
