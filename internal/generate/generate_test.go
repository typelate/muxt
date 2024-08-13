package generate_test

import (
	"bytes"
	"log"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crhntr/muxt/internal/generate"
)

func TestCommand(t *testing.T) {
	t.Run("001", func(t *testing.T) {
		dir, err := filepath.Abs(filepath.FromSlash("testdata/001/fruit"))
		require.NoError(t, err)
		logBuffer := bytes.NewBuffer(nil)
		logger := log.New(logBuffer, "", 0)
		require.NoError(t, generate.Command([]string{}, dir, logger, defaultLookupEnv(t)))

		assert.Contains(t, logBuffer.String(), ` has route for GET /fruits/{fruit}/edit`)
		assert.Contains(t, logBuffer.String(), ` has route for PATCH /fruits/{fruit} EditRow(response, request, fruit)`)
		assert.Contains(t, logBuffer.String(), ` has route for GET /farm`)

		out := bytes.NewBuffer(nil)
		cmd := exec.Command("go", "test")
		cmd.Dir = dir
		cmd.Stderr = out
		cmd.Stdout = out
		assert.NoError(t, cmd.Run(), out.String())
	})
}

func defaultLookupEnv(t *testing.T) func(name string) (string, bool) {
	return func(name string) (string, bool) {
		t.Helper()
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
}
