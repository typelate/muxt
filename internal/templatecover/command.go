package templatecover

import (
	"context"
	"os/exec"
	"slices"
	"time"
)

// Command is the go command with args, run in dir. When ctx is done the go
// command and the compilers and test binaries it started are interrupted
// together, and one that ignores the interrupt is killed after ten seconds.
//
// A nil env is the process's own environment. Otherwise PWD is set to dir
// as well: os/exec points PWD at Dir only when it supplies the environment
// itself, and the go command addresses files the way PWD says. An overlay
// keyed on the paths the loader reported is ignored when the two disagree,
// so every file in it would build unchanged.
func Command(ctx context.Context, dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "go", args...)
	interruptTogether(cmd)
	cmd.WaitDelay = 10 * time.Second
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(slices.Clip(env), "PWD="+dir)
	}
	return cmd
}
