//go:build !unix

package runner

import "os/exec"

// killGroupOnCancel leaves the default: only the shell is killed when the budget runs out.
func killGroupOnCancel(*exec.Cmd) {}
