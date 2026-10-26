// Package safepath holds the one check that keeps served files inside the
// library folder. Every adapter that opens a library file for someone else
// (the HTTP server, the thumbnail maker) goes through it.
package safepath

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrOutsideRoot means a path resolves to somewhere outside the library root.
var ErrOutsideRoot = errors.New("path resolves outside the library root")

// Inside returns the real (symlink-free) path of file, provided it is still
// inside root. This is defense in depth on top of ID lookup: even if a bad path
// ever reached the repository, or a file was swapped for a symlink after the
// scan, it could not be opened.
func Inside(root, file string) (string, error) {
	if root == "" {
		return "", errors.New("no library loaded")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	realFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		return "", fmt.Errorf("resolve file: %w", err)
	}
	rel, err := filepath.Rel(realRoot, realFile)
	if err != nil {
		return "", ErrOutsideRoot
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrOutsideRoot
	}
	return realFile, nil
}
