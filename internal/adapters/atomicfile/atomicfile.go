// Package atomicfile writes a small file so that a crash or a full disk leaves
// either the old contents or the new, never half of one. Settings, profiles and
// resume positions are all written this way.
package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Write replaces dir/name with data. The data goes to a temporary file in the same
// directory, is flushed to disk, and is renamed over the real file. The file is
// readable by its owner only, and so is the directory when it has to be created.
func Write(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		// Chmod is a no-op or refused on Windows and some filesystems; the per-user
		// directory is already private there.
		tmp.Close()
		cleanup()
		return fmt.Errorf("restrict file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write: %w", err)
	}
	if err := tmp.Sync(); err != nil { // reach the disk before the rename makes it the real file
		tmp.Close()
		cleanup()
		return fmt.Errorf("flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		cleanup()
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}
