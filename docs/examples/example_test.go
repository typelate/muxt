package examples

import (
	"bytes"
	"os/exec"
	"testing"
)

var exampleDirs = []string{"simple", "htmx-counter", "htmx-todo", "fixiproject-clock"}

func TestGoVet(t *testing.T) {
	for _, dir := range exampleDirs {
		t.Run(dir, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), "go", "vet", "./...")
			var buf bytes.Buffer
			cmd.Stderr = &buf
			cmd.Stdout = &buf
			cmd.Dir = dir
			if err := cmd.Run(); err != nil {
				t.Log(buf.String())
				t.Fatal(err)
			}
		})
	}
}

// TestRegenerate runs each example's go:generate directive against the current
// muxt source and fails if that changes template_routes.go.
func TestRegenerate(t *testing.T) {
	for _, dir := range exampleDirs {
		t.Run(dir, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), "go", "generate", "./...")
			var buf bytes.Buffer
			cmd.Stderr = &buf
			cmd.Stdout = &buf
			cmd.Dir = dir
			if err := cmd.Run(); err != nil {
				t.Log(buf.String())
				t.Fatal(err)
			}
			diff := exec.CommandContext(t.Context(), "git", "diff", "--exit-code", "--", "template_routes.go")
			var diffOut bytes.Buffer
			diff.Stderr = &diffOut
			diff.Stdout = &diffOut
			diff.Dir = dir
			if err := diff.Run(); err != nil {
				t.Log(diffOut.String())
				t.Errorf("template_routes.go in %s is stale: commit the regenerated file", dir)
			}
		})
	}
}
