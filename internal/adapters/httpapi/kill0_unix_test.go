//go:build !windows

package httpapi

import (
	"os"
	"strconv"
	"syscall"
)

// probeProcess sends signal 0 to a process: nil means it is still running.
func probeProcess(pid string) error {
	n, _ := strconv.Atoi(pid)
	p, _ := os.FindProcess(n)
	return p.Signal(syscall.Signal(0))
}
