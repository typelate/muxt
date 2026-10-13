//go:build unix

package mutation

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
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
		pid := cmd.Process.Pid
		// After WaitDelay os/exec kills only the go command. Anything in
		// the group that ignored the interrupt would be left running with
		// nothing to wait for it, so kill the group as well.
		time.AfterFunc(cmd.WaitDelay, func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
		err := syscall.Kill(-pid, syscall.SIGINT)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
