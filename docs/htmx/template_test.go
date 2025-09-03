package hypertext

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestTemplates(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "go", "run", "github.com/typelate/muxt/cmd/muxt", "check", "--receiver-type=Server")
	var buf bytes.Buffer
	cmd.Stderr = &buf
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		t.Log(buf.String())
		t.Fatal(err)
	}
}
