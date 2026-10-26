// Package settingsfile stores the user's settings as a small JSON file in the
// per-user configuration directory. It implements app.SettingsStore. Secrets
// are deliberately not stored here (see adapters/secrets).
package settingsfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

const (
	fileName = "settings.json"
	// schemaVersion lets a future release recognise and migrate an older file.
	schemaVersion = 1
)

// ErrCorrupt means the file exists but cannot be understood.
var ErrCorrupt = errors.New("settings file is corrupt")

// Store reads and writes the settings file in one directory.
type Store struct {
	dir string
}

var _ app.SettingsStore = (*Store)(nil)

// New returns a Store that keeps settings.json in dir. The directory is created
// on first save.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// file is the on-disk shape. It is separate from app.Settings so the file format
// and the in-memory type can evolve independently.
type file struct {
	Version int    `json:"version"`
	Port    int    `json:"port"`
	Name    string `json:"name"`
	// LibraryRoot is the single folder of versions before v1.0. It is read, to be
	// turned into a library, and no longer written.
	LibraryRoot  string        `json:"libraryRoot,omitempty"`
	Libraries    []libraryFile `json:"libraries,omitempty"`
	RequireToken bool          `json:"requireToken"`
	// DLNA is absent from files written before v0.5, which reads as false: off.
	DLNA bool `json:"dlna"`
}

// libraryFile is one remembered library. The ID is not stored: it is derived from
// the folder, so it cannot disagree with it.
type libraryFile struct {
	Name string `json:"name"`
	Root string `json:"root"`
}

func (s *Store) path() string { return filepath.Join(s.dir, fileName) }

// Load returns app.ErrNoSettings (wrapped) when no file exists yet. A file that
// cannot be parsed is moved aside to settings.json.bad, so the next Save does
// not destroy it, and Load returns ErrCorrupt.
func (s *Store) Load() (app.Settings, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return app.Settings{}, fmt.Errorf("%s: %w", s.path(), app.ErrNoSettings)
	}
	if err != nil {
		return app.Settings{}, fmt.Errorf("read settings: %w", err)
	}

	var f file
	dec := json.NewDecoder(bytes.NewReader(data))
	// Unknown fields are tolerated on purpose: a file written by a newer
	// version must still open in an older one.
	if err := dec.Decode(&f); err != nil {
		bad := s.path() + ".bad"
		if rerr := os.Rename(s.path(), bad); rerr != nil {
			return app.Settings{}, fmt.Errorf("%w: %v (and it could not be moved aside: %v)", ErrCorrupt, err, rerr)
		}
		return app.Settings{}, fmt.Errorf("%w: %v (moved to %s)", ErrCorrupt, err, bad)
	}
	return app.Settings{
		Port:         f.Port,
		Name:         f.Name,
		Libraries:    librariesFrom(f),
		RequireToken: f.RequireToken,
		DLNA:         f.DLNA,
	}, nil
}

// Save writes the settings atomically: the data goes to a temporary file in the
// same directory and is renamed over the real one, so a crash or full disk
// leaves either the old file or the new one, never half of one. The file is
// readable by the owner only.
func (s *Store) Save(in app.Settings) error {
	data, err := json.MarshalIndent(file{
		Version:      schemaVersion,
		Port:         in.Port,
		Name:         in.Name,
		Libraries:    librariesTo(in.Libraries),
		RequireToken: in.RequireToken,
		DLNA:         in.DLNA,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, fileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary settings file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if err := tmp.Chmod(0o600); err != nil && !isChmodUnsupported(err) {
		tmp.Close()
		cleanup()
		return fmt.Errorf("restrict settings file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write settings: %w", err)
	}
	if err := tmp.Sync(); err != nil { // reach the disk before the rename makes it the real file
		tmp.Close()
		cleanup()
		return fmt.Errorf("flush settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close settings: %w", err)
	}
	if err := os.Rename(tmpName, s.path()); err != nil {
		cleanup()
		return fmt.Errorf("replace settings file: %w", err)
	}
	return nil
}

// isChmodUnsupported tolerates filesystems (and Windows) where Chmod is a no-op
// or refused; the per-user config directory is already private there.
func isChmodUnsupported(err error) bool {
	return errors.Is(err, errors.ErrUnsupported)
}

// librariesFrom reads the list of libraries, turning the single folder of an
// older file into a one-library list. The conversion is deterministic (an ID comes
// from the folder), so loading the same old file twice gives the same result and
// nothing has to be written back first.
func librariesFrom(f file) []media.Library {
	var out []media.Library
	for _, l := range f.Libraries {
		if l.Root == "" {
			continue
		}
		lib := media.NewLibrary(l.Root)
		if l.Name != "" {
			lib.Name = l.Name
		}
		out = append(out, lib)
	}
	if len(out) == 0 && f.LibraryRoot != "" {
		out = append(out, media.NewLibrary(f.LibraryRoot))
	}
	return out
}

func librariesTo(libs []media.Library) []libraryFile {
	out := make([]libraryFile, len(libs))
	for i, l := range libs {
		out[i] = libraryFile{Name: l.Name, Root: l.Root}
	}
	return out
}
