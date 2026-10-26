//go:build !windows

package portproc

import "os/exec"

// hideWindow is only needed on Windows.
func hideWindow(*exec.Cmd) {}
