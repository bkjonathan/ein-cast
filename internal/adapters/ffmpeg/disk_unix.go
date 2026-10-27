//go:build !windows

package ffmpeg

import (
	"fmt"
	"syscall"
)

// freeBytes is how much an ordinary program may still write on the disk that
// holds dir.
func freeBytes(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, fmt.Errorf("look at the free space of %s: %w", dir, err)
	}
	// Bavail, not Bfree: the part reserved for the system is not ours to fill.
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
