//go:build unix

package mutation

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// interruptTogether puts the go command in a process group of its own and
// has cancelling it interrupt the whole group.
//
// Killing only the go command would leave the compilers and test binaries
// it started running with nothing to wait for them, and would leave its
// work directory behind. Interrupted, as a terminal would interrupt it,
// the go command removes its work directory on the way out, and the test
// binaries in its group stop with it.
func interruptTogether(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
