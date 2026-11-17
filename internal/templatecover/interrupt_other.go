//go:build !unix

package templatecover

import "os/exec"

// interruptTogether leaves cancelling to os/exec, which kills the go
// command. There is no process group to interrupt here.
func interruptTogether(*exec.Cmd) {}
