//go:build unix

package mutation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoTestStopsWithItsContext states that cancelling a go test run stops
// the test binary it started, not only the go command: a run interrupted
// mid-mutant must not leave a suite running with nothing waiting for it.
func TestGoTestStopsWithItsContext(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{
		"go.mod": goMod,
		"sleep_test.go": `package server

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func TestSleep(t *testing.T) {
	if err := os.WriteFile("pid", []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Minute)
}
`,
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tester := goTest{dir: dir, packages: []string{"./..."}, env: goEnv()}
	done := make(chan error, 1)
	go func() {
		_, err := tester.baseline(ctx)
		done <- err
	}()

	var pid int
	require.Eventually(t, func() bool {
		b, err := os.ReadFile(filepath.Join(dir, "pid"))
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
		return err == nil
	}, time.Minute, 50*time.Millisecond, "the test binary never started")

	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled, "baseline after cancelling")
	case <-time.After(30 * time.Second):
		require.Fail(t, "go test was still running 30s after its context was cancelled")
	}
	assert.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	}, 10*time.Second, 50*time.Millisecond, "test binary %d outlived the go test that started it", pid)
}
