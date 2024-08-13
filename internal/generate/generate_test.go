package generate_test

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crhntr/muxt/internal/generate"
)

// TestCommand001 is the initial iteration for the generate command
// It is factored to allow debugging.
// After generating the handler, it runs go test.
func TestCommand001(t *testing.T) {

	lookupEnv := func(name string) (string, bool) {
		switch name {
		case "GOLINE":
			return "14", true
		case "GOFILE":
			return "execute.go", true
		case "GOPACKAGE":
			return "fruit", true
		default:
			t.Errorf("unexpected LookupEnv call %q", name)
			return "", false
		}
	}

	dir, err := filepath.Abs(filepath.FromSlash("testdata/001/fruit"))
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(dir, "template_routes.go")))
	logBuffer := bytes.NewBuffer(nil)
	logger := log.New(logBuffer, "", 0)
	require.NoError(t, generate.Command([]string{}, dir, logger, lookupEnv))

	assert.Contains(t, logBuffer.String(), ` has route for GET /fruits/{fruit}/edit`)
	assert.Contains(t, logBuffer.String(), ` has route for PATCH /fruits/{fruit} EditRow(response, request, fruit)`)
	assert.Contains(t, logBuffer.String(), ` has route for GET /farm`)

	out := bytes.NewBuffer(nil)
	cmd := exec.Command("go", "test")
	cmd.Dir = dir
	cmd.Stderr = out
	cmd.Stdout = out
	assert.NoError(t, cmd.Run(), out.String())
}
