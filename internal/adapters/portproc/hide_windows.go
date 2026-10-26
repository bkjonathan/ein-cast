package portproc

import (
	"os/exec"
	"syscall"
)

// createNoWindow is the Windows process flag that keeps a console program from
// opening a console window of its own.
const createNoWindow = 0x08000000

// hideWindow stops a console window flashing up for every lookup.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
