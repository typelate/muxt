//go:build unix

package mutation

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInterruptTogetherEndsAChildThatIgnoresTheInterrupt states that
// cancelling a run leaves nothing behind: a process in the go command's
// group that ignores SIGINT is killed once WaitDelay has passed, not
// orphaned.
func TestInterruptTogetherEndsAChildThatIgnoresTheInterrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", "trap '' INT; sleep 300 & echo $!; wait")
	interruptTogether(cmd)
	cmd.WaitDelay = 200 * time.Millisecond
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err, "reading the child's pid")
	child, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err, "the child's pid in %q", line)
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL) })

	cancel()
	_ = cmd.Wait()

	assert.Eventually(t, func() bool {
		return syscall.Kill(child, 0) == syscall.ESRCH
	}, 5*time.Second, 50*time.Millisecond, "process %d that ignored SIGINT is still running", child)
}
