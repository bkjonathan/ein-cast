//go:build !windows

package relaunch

import (
	"errors"
	"syscall"
)

// alive reports whether a process with this id exists. Signal 0 delivers
// nothing; it only checks. "Not allowed" still means the process is there.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
