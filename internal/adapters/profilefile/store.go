// Package profilefile stores the profiles, with their PIN hashes, as a small JSON
// file next to settings.json. It implements app.ProfileStore (ADR 0027).
package profilefile

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
	"eincast/internal/domain/profile"
)

const (
	fileName = "profiles.json"
	// schemaVersion lets a future release recognise and migrate an older file.
	schemaVersion = 1
)

// ErrCorrupt means the file exists but cannot be understood.
var ErrCorrupt = errors.New("profiles file is corrupt")

// Store reads and writes the profiles file in one directory.
type Store struct{ dir string }

var _ app.ProfileStore = (*Store)(nil)

// New returns a Store that keeps profiles.json in dir.
func New(dir string) *Store { return &Store{dir: dir} }

// file is the on-disk shape, apart from the domain type so the two can change
// independently. []byte fields are stored as base64 by encoding/json.
type file struct {
	Version  int          `json:"version"`
	Profiles []profileRec `json:"profiles"`
}

type profileRec struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Color         string   `json:"color"`
	AllLibraries  bool     `json:"allLibraries"`
	Libraries     []string `json:"libraries,omitempty"`
	Kinds         []string `json:"kinds,omitempty"`
	HiddenFolders []string `json:"hiddenFolders,omitempty"`
	PINHash       []byte   `json:"pinHash,omitempty"`
	PINSalt       []byte   `json:"pinSalt,omitempty"`
	PINVersion    int      `json:"pinVersion"`
}

func (s *Store) path() string { return filepath.Join(s.dir, fileName) }

// Load returns app.ErrNoProfiles (wrapped) when there is no file. A file that
// cannot be parsed is moved aside to profiles.json.bad, so the next Save does not
// destroy it, and Load returns ErrCorrupt.
func (s *Store) Load() ([]app.ProfileRecord, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", s.path(), app.ErrNoProfiles)
	}
	if err != nil {
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	var f file
	// Unknown fields are tolerated: a file from a newer version must still open.
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&f); err != nil {
		bad := s.path() + ".bad"
		if rerr := os.Rename(s.path(), bad); rerr != nil {
			return nil, fmt.Errorf("%w: %v (and it could not be moved aside: %v)", ErrCorrupt, err, rerr)
		}
		return nil, fmt.Errorf("%w: %v (moved to %s)", ErrCorrupt, err, bad)
	}
	out := make([]app.ProfileRecord, len(f.Profiles))
	for i, r := range f.Profiles {
		out[i] = toRecord(r)
	}
	return out, nil
}

// Save writes the profiles atomically, readable by the owner only.
func (s *Store) Save(recs []app.ProfileRecord) error {
	f := file{Version: schemaVersion, Profiles: make([]profileRec, len(recs))}
	for i, r := range recs {
		f.Profiles[i] = fromRecord(r)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profiles: %w", err)
	}
	if err := atomicfile.Write(s.dir, fileName, append(data, '\n')); err != nil {
		return fmt.Errorf("write profiles: %w", err)
	}
	return nil
}

func toRecord(r profileRec) app.ProfileRecord {
	p := profile.Profile{
		ID: profile.ID(r.ID), Name: r.Name, Color: r.Color, AllLibraries: r.AllLibraries,
		HiddenFolders: r.HiddenFolders,
	}
	for _, l := range r.Libraries {
		p.Libraries = append(p.Libraries, media.LibraryID(l))
	}
	for _, k := range r.Kinds {
		p.Kinds = append(p.Kinds, media.Kind(k))
	}
	return app.ProfileRecord{Profile: p, PINHash: r.PINHash, PINSalt: r.PINSalt, Version: r.PINVersion}
}

func fromRecord(r app.ProfileRecord) profileRec {
	p := r.Profile
	out := profileRec{
		ID: string(p.ID), Name: p.Name, Color: p.Color, AllLibraries: p.AllLibraries,
		HiddenFolders: p.HiddenFolders, PINHash: r.PINHash, PINSalt: r.PINSalt, PINVersion: r.Version,
	}
	for _, l := range p.Libraries {
		out.Libraries = append(out.Libraries, string(l))
	}
	for _, k := range p.Kinds {
		out.Kinds = append(out.Kinds, string(k))
	}
	return out
}
