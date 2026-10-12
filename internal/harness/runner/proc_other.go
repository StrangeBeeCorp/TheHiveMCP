//go:build !unix

package runner

import "os/exec"

// killGroupOnCancel leaves the default: only the shell is killed when the budget runs out or the
// run is interrupted.
func killGroupOnCancel(*exec.Cmd) {}
