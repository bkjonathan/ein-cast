package relaunch

import "syscall"

// waitTimeout is what WaitForSingleObject returns when the process has not ended.
const waitTimeout = 0x00000102

// alive reports whether a process with this id is still running. A process that
// cannot be opened is taken as gone: it either ended or is not ours to wait for.
func alive(pid int) bool {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	event, err := syscall.WaitForSingleObject(h, 0)
	return err == nil && event == waitTimeout
}
