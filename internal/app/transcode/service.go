// Package transcode is the use case behind playing a video a device cannot
// play as it is: convert it to HLS once, share that conversion between requests,
// and clean up when nobody is watching.
package transcode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// ErrBusy means all conversions are in use by other videos that are being
// watched. The caller can try again shortly.
var ErrBusy = errors.New("too many videos are being converted at once")

// ErrNotStarted means a segment was asked for although no conversion of that
// video is running, typically because it was cleaned up after sitting idle.
var ErrNotStarted = errors.New("no conversion of this video is running")

// Items is the part of the library this service reads. Declared here, at the
// consumer.
type Items interface {
	Get(id media.ID) (media.Item, error)
}

// Options tune a Service.
type Options struct {
	// MaxStreams is how many videos may be converted at once. Each is an ffmpeg
	// process that may use a whole CPU core or more.
	MaxStreams int
	// IdleAfter is how long a conversion with no requests is kept. Players fetch
	// a segment every few seconds, so silence means they are gone (or paused for
	// a long time, in which case the next request starts over).
	IdleAfter time.Duration
	// ReplaceAfter is how long a conversion must have been idle before a new
	// video may take its place when all slots are busy. It protects a video
	// someone is watching from being cut off by someone starting another.
	ReplaceAfter time.Duration
	// ReadyTimeout caps the wait for the first segments.
	ReadyTimeout time.Duration
	// SweepEvery is how often idle conversions are looked for.
	SweepEvery time.Duration
}

// DefaultOptions suit a home computer.
var DefaultOptions = Options{
	MaxStreams:   2,
	IdleAfter:    2 * time.Minute,
	ReplaceAfter: 30 * time.Second,
	ReadyTimeout: 60 * time.Second,
	SweepEvery:   15 * time.Second,
}

// Service owns the running conversions, one per video.
type Service struct {
	items Items
	tc    app.Transcoder
	log   *slog.Logger
	opts  Options
	now   func() time.Time // a field so tests control time

	mu       sync.Mutex
	sessions map[media.ID]*session
}

// session is one video's conversion and when it was last used.
type session struct {
	stream  app.HLSStream
	modTime time.Time // of the source file; a changed file needs a new conversion
	lastUse time.Time
	started time.Time
}

// failed reports whether the conversion has ended with an error.
func (s *session) failed() bool {
	select {
	case <-s.stream.Done():
		return s.stream.Err() != nil
	default:
		return false
	}
}

// NewService builds a Service from its ports.
func NewService(items Items, tc app.Transcoder, log *slog.Logger, opts Options) *Service {
	opts.MaxStreams = max(opts.MaxStreams, 1)
	return &Service{
		items:    items,
		tc:       tc,
		log:      log.With(slog.String("source", "transcode")),
		opts:     opts,
		now:      time.Now,
		sessions: make(map[media.ID]*session),
	}
}

// Available reports whether videos can be converted on this computer.
func (s *Service) Available() bool {
	return s.tc.Available()
}

// Playlist returns the HLS playlist of a video, starting the conversion if none
// is running and waiting until playback can begin. Errors: media.ErrNotFound,
// app.ErrTranscodingUnavailable, ErrBusy, and app.ErrNotConvertible (wrapped,
// with the reason) when ffmpeg could not do it.
func (s *Service) Playlist(ctx context.Context, id media.ID) (io.ReadSeekCloser, error) {
	sess, err := s.acquire(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.waitReady(ctx, sess); err != nil {
		return nil, err
	}
	return sess.stream.Open("index.m3u8")
}

// File returns one output file (a segment, or the playlist again) of a
// conversion that is already running. It never starts one: a player asking for
// a segment of a conversion that was cleaned up should fetch the playlist again.
func (s *Service) File(id media.ID, name string) (io.ReadSeekCloser, error) {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	if ok && !sess.failed() {
		sess.lastUse = s.now()
	}
	s.mu.Unlock()
	if !ok {
		return nil, ErrNotStarted
	}
	return sess.stream.Open(name)
}

// acquire returns the video's running conversion, starting one if needed.
func (s *Service) acquire(ctx context.Context, id media.ID) (*session, error) {
	it, err := s.items.Get(id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.sessions[id]; ok {
		switch {
		case !sess.modTime.Equal(it.ModTime):
			s.drop(id, sess, "the file changed") // converted from an older version
		case sess.failed():
			// Handed back as it is until it is cleaned up, so a file ffmpeg cannot
			// read costs one process, not one per request.
			return sess, nil
		default:
			sess.lastUse = s.now()
			return sess, nil
		}
	}

	if !s.tc.Available() {
		return nil, app.ErrTranscodingUnavailable
	}
	if len(s.sessions) >= s.opts.MaxStreams && !s.makeRoom() {
		return nil, ErrBusy
	}
	stream, err := s.tc.Start(ctx, app.TranscodeRequest{Root: it.Root, Path: it.Path})
	if err != nil {
		return nil, fmt.Errorf("start converting %q: %w", it.Name, err)
	}
	now := s.now()
	sess := &session{stream: stream, modTime: it.ModTime, lastUse: now, started: now}
	s.sessions[id] = sess
	s.log.Info("converting for streaming", slog.String("name", it.Name), slog.Int("running", len(s.sessions)))
	return sess, nil
}

// makeRoom frees a slot by stopping the conversion that has been idle longest,
// if it has been idle long enough. Failed ones go first: they serve nobody.
// Must be called with mu held.
func (s *Service) makeRoom() bool {
	var victim media.ID
	var oldest time.Time
	found := false
	for id, sess := range s.sessions {
		idle := s.now().Sub(sess.lastUse)
		if !sess.failed() && idle < s.opts.ReplaceAfter {
			continue
		}
		if !found || sess.lastUse.Before(oldest) {
			victim, oldest, found = id, sess.lastUse, true
		}
	}
	if !found {
		return false
	}
	s.drop(victim, s.sessions[victim], "making room for another video")
	return true
}

// drop stops a conversion and forgets it. Must be called with mu held.
func (s *Service) drop(id media.ID, sess *session, why string) {
	sess.stream.Stop()
	delete(s.sessions, id)
	s.log.Info("stopped converting", slog.String("id", string(id)), slog.String("why", why))
}

func (s *Service) waitReady(ctx context.Context, sess *session) error {
	timeout := time.NewTimer(s.opts.ReadyTimeout)
	defer timeout.Stop()
	select {
	case <-sess.stream.Ready():
		return nil
	case <-sess.stream.Done():
		// Done can race with Ready for a short video; Ready wins if it is set.
		select {
		case <-sess.stream.Ready():
			return nil
		default:
		}
		if err := sess.stream.Err(); err != nil {
			return fmt.Errorf("%w: %v", app.ErrNotConvertible, err)
		}
		return app.ErrNotConvertible
	case <-timeout.C:
		return fmt.Errorf("%w: it took too long to start", app.ErrNotConvertible)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run cleans up idle conversions until ctx ends, then stops them all. Run it in
// its own goroutine, owned by whoever owns ctx.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.opts.SweepEvery)
	defer ticker.Stop()
	defer s.StopAll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep()
		}
	}
}

// sweep stops conversions nobody has asked for lately.
func (s *Service) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		if s.now().Sub(sess.lastUse) >= s.opts.IdleAfter {
			s.drop(id, sess, "idle")
		}
	}
}

// StopAll stops every conversion and deletes its files. It is called when the
// app quits, and when the server stops.
func (s *Service) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		s.drop(id, sess, "shutting down")
	}
}

// Running is how many conversions exist, for the UI and tests.
func (s *Service) Running() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}
