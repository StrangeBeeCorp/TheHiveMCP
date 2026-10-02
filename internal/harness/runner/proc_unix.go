//go:build unix

package runner

import (
	"os/exec"
	"syscall"
	"time"
)

// waitDelay is how long a cancelled command's process group has to stop on SIGTERM before it gets
// SIGKILL, and how long it may keep its output pipes open.
const waitDelay = 5 * time.Second

// killGroupOnCancel runs the command in its own process group and stops the whole group when the
// gate's budget runs out or the run is interrupted, so the tools a shell command started stop with
// it: SIGTERM first, which docker run passes on to its container, then SIGKILL.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		group := -cmd.Process.Pid

		time.AfterFunc(waitDelay, func() { _ = syscall.Kill(group, syscall.SIGKILL) })

		return syscall.Kill(group, syscall.SIGTERM)
	}
	cmd.WaitDelay = waitDelay
}
