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

	"eincast/internal/adapters/atomicfile"
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
	// The next four are absent from files written before v1.1, which keeps what
	// those installs always did: nothing starts by itself, the window closes the
	// app. KeepAwake is a pointer because absent means "on" there: an old install
	// gets the benign default, not a silent off.
	AutoStart   bool   `json:"autoStart"`
	KeepAwake   *bool  `json:"keepAwake,omitempty"`
	OpenAtLogin bool   `json:"openAtLogin"`
	HideOnClose bool   `json:"hideOnClose"`
	FFmpegPath  string `json:"ffmpegPath,omitempty"`
	// HardwareEncoding is a pointer for the same reason as KeepAwake: absent from
	// files written before it existed, which means "on".
	HardwareEncoding *bool `json:"hardwareEncoding,omitempty"`
}

// libraryFile is one remembered library. The ID is not stored: it is derived from
// the folder, so it cannot disagree with it.
type libraryFile struct {
	Name string `json:"name"`
	Root string `json:"root"`
	// HiddenFromTVs is absent from older files, which reads as false: visible.
	HiddenFromTVs bool `json:"hiddenFromTVs,omitempty"`
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
		AutoStart:    f.AutoStart,
		KeepAwake:    f.KeepAwake == nil || *f.KeepAwake,
		OpenAtLogin:  f.OpenAtLogin,
		HideOnClose:  f.HideOnClose,
		FFmpegPath:   f.FFmpegPath,

		HardwareEncoding: f.HardwareEncoding == nil || *f.HardwareEncoding,
	}, nil
}

// Save writes the settings atomically and readable by the owner only (see
// atomicfile).
func (s *Store) Save(in app.Settings) error {
	keepAwake, hardware := in.KeepAwake, in.HardwareEncoding
	data, err := json.MarshalIndent(file{
		Version:      schemaVersion,
		Port:         in.Port,
		Name:         in.Name,
		Libraries:    librariesTo(in.Libraries),
		RequireToken: in.RequireToken,
		DLNA:         in.DLNA,
		AutoStart:    in.AutoStart,
		KeepAwake:    &keepAwake,
		OpenAtLogin:  in.OpenAtLogin,
		HideOnClose:  in.HideOnClose,
		FFmpegPath:   in.FFmpegPath,

		HardwareEncoding: &hardware,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	if err := atomicfile.Write(s.dir, fileName, data); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
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
		lib.HiddenFromTVs = l.HiddenFromTVs
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
		out[i] = libraryFile{Name: l.Name, Root: l.Root, HiddenFromTVs: l.HiddenFromTVs}
	}
	return out
}
