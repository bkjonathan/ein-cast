package media

import "strings"

// Ignored reports whether a file or folder name is never library content: a
// hidden entry (name starts with "."), which also covers macOS "._*" AppleDouble
// files and ".DS_Store", or the metadata Windows, macOS archives and Synology
// NAS boxes leave behind. The comparison ignores case because Windows does.
//
// The scanner and the folder watcher both ask this, so a file the scan skips
// can never wake the watcher.
func Ignored(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "thumbs.db", "desktop.ini", "$recycle.bin", "system volume information",
		"__macosx", "@eadir":
		return true
	default:
		return false
	}
}
