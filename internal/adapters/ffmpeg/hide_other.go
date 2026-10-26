//go:build !windows

package ffmpeg

import "os/exec"

// hideWindow is only needed on Windows.
func hideWindow(*exec.Cmd) {}
