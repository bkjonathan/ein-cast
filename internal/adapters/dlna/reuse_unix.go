//go:build !windows

package dlna

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// shareAddress lets this socket bind a port that other programs already listen
// on. Port 1900 is shared by design: every UPnP device and control point on a
// computer listens on it, and the system hands each multicast packet to all.
func shareAddress(_, _ string, c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		if serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); serr != nil {
			return
		}
		// Required on macOS and the BSDs for a second listener; harmless on Linux.
		serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
	if err != nil {
		return err
	}
	return serr
}
