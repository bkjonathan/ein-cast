package relaunch

import (
	"os/exec"
	"syscall"
)

// detachedProcess is the Windows flag for a process with no console of its own.
const detachedProcess = 0x00000008

// detach lets the new process outlive this one.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP}
}
