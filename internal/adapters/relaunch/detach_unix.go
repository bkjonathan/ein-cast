//go:build !windows

package relaunch

import (
	"os/exec"
	"syscall"
)

// detach puts the new process in a session of its own, so it is not taken down
// with this one.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
