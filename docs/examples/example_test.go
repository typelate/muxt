package examples

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestGoVet(t *testing.T) {
	for _, dir := range []string{"counter-htmx", "simple", "todo-htmx"} {
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
