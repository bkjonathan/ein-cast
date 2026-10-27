// Package eraser deletes EinCast's own folders: the settings directory and the
// cache directory. It implements app.DataEraser (ADR 0034).
package eraser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"eincast/internal/app"
)

// Eraser removes whole directories, and nothing else.
type Eraser struct {
	dirs []string
	home string // never removed, whatever it is asked
}

var _ app.DataEraser = (*Eraser)(nil)

// New returns an Eraser for dirs. They are the folders EinCast made for itself
// (<config>/EinCast and <cache>/EinCast), never the media folders.
func New(dirs ...string) *Eraser {
	home, _ := os.UserHomeDir() // empty when unknown; the check then has nothing to compare
	return &Eraser{dirs: dirs, home: home}
}

// Erase removes every directory it was given. A directory that is already gone is
// fine. One that fails does not stop the rest.
func (e *Eraser) Erase() error {
	var errs []error
	for _, dir := range e.dirs {
		if err := e.refuse(dir); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", dir, err))
		}
	}
	return errors.Join(errs...)
}

// refuse is the last line of defence against a wrong path reaching RemoveAll: an
// empty or relative path, the root of a disk, or the home folder would destroy
// far more than EinCast's own files.
func (e *Eraser) refuse(dir string) error {
	if dir == "" || !filepath.IsAbs(dir) {
		return fmt.Errorf("refusing to erase %q: not an absolute path", dir)
	}
	clean := filepath.Clean(dir)
	if filepath.Dir(clean) == clean {
		return fmt.Errorf("refusing to erase %q: it is the root of a disk", dir)
	}
	if e.home != "" && clean == filepath.Clean(e.home) {
		return fmt.Errorf("refusing to erase %q: it is the home folder", dir)
	}
	return nil
}
