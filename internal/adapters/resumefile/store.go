// Package resumefile stores resume positions as a small JSON file next to
// settings.json. It implements app.ResumeStore (ADR 0027).
package resumefile

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
	fileName      = "resume.json"
	schemaVersion = 1
)

// ErrCorrupt means the file exists but cannot be understood.
var ErrCorrupt = errors.New("resume file is corrupt")

// Store reads and writes the resume file in one directory.
type Store struct{ dir string }

var _ app.ResumeStore = (*Store)(nil)

// New returns a Store that keeps resume.json in dir.
func New(dir string) *Store { return &Store{dir: dir} }

type file struct {
	Version  int                            `json:"version"`
	Profiles map[string]map[string]position `json:"profiles"`
}

type position struct {
	T int   `json:"t"`
	D int   `json:"d"`
	M int64 `json:"m"`
	S int64 `json:"s"`
}

func (s *Store) path() string { return filepath.Join(s.dir, fileName) }

// Load returns an empty map when nothing is saved. A file that cannot be parsed
// is moved aside to resume.json.bad and reported as ErrCorrupt.
func (s *Store) Load() (map[profile.ID]map[media.ID]app.ResumePosition, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return map[profile.ID]map[media.ID]app.ResumePosition{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read resume positions: %w", err)
	}
	var f file
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&f); err != nil {
		bad := s.path() + ".bad"
		if rerr := os.Rename(s.path(), bad); rerr != nil {
			return nil, fmt.Errorf("%w: %v (and it could not be moved aside: %v)", ErrCorrupt, err, rerr)
		}
		return nil, fmt.Errorf("%w: %v (moved to %s)", ErrCorrupt, err, bad)
	}
	out := make(map[profile.ID]map[media.ID]app.ResumePosition, len(f.Profiles))
	for p, items := range f.Profiles {
		m := make(map[media.ID]app.ResumePosition, len(items))
		for id, pos := range items {
			m[media.ID(id)] = app.ResumePosition{T: pos.T, D: pos.D, M: pos.M, S: pos.S}
		}
		out[profile.ID(p)] = m
	}
	return out, nil
}

// Save writes the positions atomically, readable by the owner only.
func (s *Store) Save(data map[profile.ID]map[media.ID]app.ResumePosition) error {
	f := file{Version: schemaVersion, Profiles: make(map[string]map[string]position, len(data))}
	for p, items := range data {
		m := make(map[string]position, len(items))
		for id, pos := range items {
			m[string(id)] = position{T: pos.T, D: pos.D, M: pos.M, S: pos.S}
		}
		f.Profiles[string(p)] = m
	}
	b, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("encode resume positions: %w", err)
	}
	if err := atomicfile.Write(s.dir, fileName, append(b, '\n')); err != nil {
		return fmt.Errorf("write resume positions: %w", err)
	}
	return nil
}
