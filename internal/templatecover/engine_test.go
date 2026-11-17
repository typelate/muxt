package templatecover_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/overlay"
	"github.com/typelate/muxt/internal/templatecover"
)

// TestEngineFlags states the go test flags that put the engine in place.
func TestEngineFlags(t *testing.T) {
	e := templatecover.Engine{Files: overlay.Files{ModFile: "/s/go.mod", Overlay: "/s/overlay.json"}, CoverDir: "/s/profiles"}
	assert.Equal(t, []string{"-mod=mod", "-modfile=/s/go.mod", "-overlay=/s/overlay.json"}, e.Flags())
}

// TestEngineEnvironNamesTheProfileDirectory states that the environment
// keeps what the caller had and adds the directory test binaries write to,
// without writing into the caller's slice.
func TestEngineEnvironNamesTheProfileDirectory(t *testing.T) {
	e := templatecover.Engine{CoverDir: "/s/profiles"}
	env := make([]string, 1, 4)
	env[0] = "A=1"

	got := e.Environ(env)

	assert.Equal(t, []string{"A=1", "MUXT_TEMPLATE_COVER=/s/profiles"}, got)
	assert.Equal(t, []string{"A=1"}, env, "the caller's environment")
	assert.Equal(t, "", env[:2][1], "the caller's spare capacity is untouched")
}

// TestUnavailableErrorIsItsCause states that the message is the cause's,
// byte for byte, and that the cause stays reachable.
func TestUnavailableErrorIsItsCause(t *testing.T) {
	cause := errors.New("go.work is in use")
	err := error(&templatecover.UnavailableError{Err: cause})

	assert.Equal(t, "go.work is in use", err.Error())
	assert.ErrorIs(t, err, cause)
}
