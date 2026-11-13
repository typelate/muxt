package composite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/composite"
)

// TestSeveralPackagesErrorMessage states that a command that tests one
// package, run where a muxt.json lists several, names the file, the command and
// the directories to pass, on one line.
func TestSeveralPackagesErrorMessage(t *testing.T) {
	err := &composite.SeveralPackagesError{
		Path:    "/m/muxt.json",
		Command: "test-template-mutations",
		Dirs:    []string{"contacts", "inventory", "tasks"},
	}

	assert.Equal(t, "/m/muxt.json: test-template-mutations tests one package; pass one of: contacts inventory tasks", err.Error())
}
