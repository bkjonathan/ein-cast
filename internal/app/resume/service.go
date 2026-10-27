// Package resume remembers where each profile stopped in each title, so
// "Continue watching" follows a person from the phone to the TV (ADR 0027).
package resume

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
)

const (
	// maxPerProfile bounds one profile's positions. The oldest go first: nobody
	// needs to resume something they last watched two thousand titles ago.
	maxPerProfile = 2000
	// maxDuration is the longest title a position is kept for (a week).
	maxDuration = 7 * 24 * 3600

	// DefaultDelay is how long writes are gathered before the file is rewritten. A
	// position is saved every few seconds while something plays; the disk does not
	// need every one.
	DefaultDelay = 2 * time.Second
)

// ErrInvalid means a position that cannot be real: negative, past the end, or of
// an impossible length.
var ErrInvalid = errors.New("invalid resume position")

// Service keeps the positions in memory and writes them out in batches. It is safe
// for concurrent use.
type Service struct {
	store app.ResumeStore
	log   *slog.Logger
	now   func() time.Time
	delay time.Duration

	mu    sync.Mutex
	data  map[profile.ID]map[media.ID]app.ResumePosition
	timer *time.Timer // set while a write is waiting
}

// NewService returns an empty Service; call Load at startup and Close at exit.
func NewService(store app.ResumeStore, log *slog.Logger) *Service {
	return &Service{
		store: store,
		log:   log.With(slog.String("source", "resume")),
		now:   time.Now,
		delay: DefaultDelay,
		data:  map[profile.ID]map[media.ID]app.ResumePosition{},
	}
}

// Load reads the saved positions. It never fails: losing resume positions is a
// nuisance, and an app that will not start over them is worse.
func (s *Service) Load() {
	data, err := s.store.Load()
	if err != nil {
		s.log.Warn("could not use the saved resume positions; starting with none", slog.String("error", err.Error()))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if data != nil {
		s.data = data
	}
}

// Get returns a profile's positions. The map is a copy.
func (s *Service) Get(p profile.ID) map[media.ID]app.ResumePosition {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[media.ID]app.ResumePosition, len(s.data[p]))
	for id, pos := range s.data[p] {
		out[id] = pos
	}
	return out
}

// Put records where p stopped in id. t and d are seconds; modTime is the file's
// modification time in milliseconds. The save time is the server's clock, not the
// device's, so devices with wrong clocks still order "Continue watching" sensibly.
func (s *Service) Put(p profile.ID, id media.ID, t, d int, modTime int64) error {
	if d < 1 || d > maxDuration || t < 0 || t > d {
		return fmt.Errorf("%w: %d of %d seconds", ErrInvalid, t, d)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.data[p]
	if m == nil {
		m = map[media.ID]app.ResumePosition{}
		s.data[p] = m
	}
	m[id] = app.ResumePosition{T: t, D: d, M: modTime, S: s.now().UnixMilli()}
	if len(m) > maxPerProfile {
		dropOldest(m)
	}
	s.scheduleLocked()
	return nil
}

// dropOldest removes the least recently saved position.
func dropOldest(m map[media.ID]app.ResumePosition) {
	var oldest media.ID
	var at int64 = 1<<63 - 1
	for id, pos := range m {
		if pos.S < at {
			oldest, at = id, pos.S
		}
	}
	delete(m, oldest)
}

// Delete forgets one position, for a title that was finished or started over.
func (s *Service) Delete(p profile.ID, id media.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[p][id]; !ok {
		return
	}
	delete(s.data[p], id)
	s.scheduleLocked()
}

// Forget removes everything a profile has, when the profile is deleted.
func (s *Service) Forget(p profile.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[p]; !ok {
		return
	}
	delete(s.data, p)
	s.scheduleLocked()
}

// scheduleLocked arranges one write after the delay, unless one is waiting.
func (s *Service) scheduleLocked() {
	if s.timer != nil {
		return
	}
	s.timer = time.AfterFunc(s.delay, func() {
		s.mu.Lock()
		s.timer = nil
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			s.log.Warn("could not save resume positions", slog.String("error", err.Error()))
		}
	})
}

func (s *Service) saveLocked() error {
	snapshot := make(map[profile.ID]map[media.ID]app.ResumePosition, len(s.data))
	for p, m := range s.data {
		if len(m) == 0 {
			continue
		}
		snapshot[p] = cloneMap(m)
	}
	return s.store.Save(snapshot)
}

func cloneMap(m map[media.ID]app.ResumePosition) map[media.ID]app.ResumePosition {
	out := make(map[media.ID]app.ResumePosition, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Close writes anything still waiting. Call it when the app quits, so the last
// few seconds of watching are not lost.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer == nil {
		return nil
	}
	s.timer.Stop()
	s.timer = nil
	return s.saveLocked()
}

// Newest returns ids ordered by save time, most recent first. It is the order
// "Continue watching" shows.
func Newest(m map[media.ID]app.ResumePosition) []media.ID {
	ids := make([]media.ID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b media.ID) int {
		if d := m[b].S - m[a].S; d != 0 {
			if d > 0 {
				return 1
			}
			return -1
		}
		return slices.Compare([]byte(a), []byte(b))
	})
	return ids
}
