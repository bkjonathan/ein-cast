//go:build windows

package dlna

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// shareAddress lets this socket bind a port that other programs already listen
// on. Port 1900 is shared by design: Windows' own SSDP service and every UPnP
// program listen on it, and the system hands each multicast packet to all.
func shareAddress(_, _ string, c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		serr = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
	})
	if err != nil {
		return err
	}
	return serr
}
