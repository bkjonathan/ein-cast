//go:build windows

package ffmpeg

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// freeBytes is how much the signed-in person may still write on the disk that
// holds dir (their quota, if there is one, not the whole volume).
func freeBytes(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, fmt.Errorf("look at the free space of %s: %w", dir, err)
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, fmt.Errorf("look at the free space of %s: %w", dir, err)
	}
	return free, nil
}
