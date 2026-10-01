//go:build unix

package runner

import (
	"os/exec"
	"syscall"
	"time"
)

// waitDelay bounds how long a cancelled command may keep its output pipes open.
const waitDelay = 5 * time.Second

// killGroupOnCancel runs the command in its own process group and kills the whole group when the
// gate's budget runs out, so the tools a shell command started stop with it.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = waitDelay
}
