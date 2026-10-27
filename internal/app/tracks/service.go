// Package tracks is the use case behind the audio and subtitle tracks stored
// inside a video: list them, and read a subtitle track out as WebVTT. Nothing is
// read ahead of time; opening a video's page costs one quick look at it.
package tracks

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// version is part of every cache key. Raise it when extraction should give
// different text, so old results are made again.
const version = 1

// maxRemembered bounds the remembered track lists. A library has thousands of
// films and a person looks at a few; when this many have been seen, the list
// starts over, which only costs a quick look at the files again.
const maxRemembered = 2048

// Items is the part of the library this service reads. Declared here, at the
// consumer.
type Items interface {
	Get(id media.ID) (media.Item, error)
}

// Options tune a Service.
type Options struct {
	// Parallel is how many subtitle tracks are extracted at once. Each one reads
	// a whole, possibly very large, file.
	Parallel int
}

// DefaultOptions suit a home computer.
var DefaultOptions = Options{Parallel: 2}

// Service lists and extracts the tracks inside videos.
type Service struct {
	items   Items
	prober  app.StreamProber
	extract app.SubtitleExtractor
	cache   app.SubtitleCache
	log     *slog.Logger
	slots   chan struct{} // a token per running extraction

	mu       sync.Mutex
	seen     map[seenKey]app.Streams
	inflight map[string]*job
}

// seenKey names a video as it is now: an edited file is looked at again.
type seenKey struct {
	id      media.ID
	modTime time.Time
	size    int64
}

// job is one extraction that several requests may wait on.
type job struct {
	done chan struct{} // closed once data and err are set
	data []byte
	err  error
}

// NewService builds a Service from its ports.
func NewService(items Items, prober app.StreamProber, extract app.SubtitleExtractor, cache app.SubtitleCache, log *slog.Logger, opts Options) *Service {
	opts.Parallel = max(opts.Parallel, 1)
	return &Service{
		items:    items,
		prober:   prober,
		extract:  extract,
		cache:    cache,
		log:      log.With(slog.String("source", "tracks")),
		slots:    make(chan struct{}, opts.Parallel),
		seen:     make(map[seenKey]app.Streams),
		inflight: make(map[string]*job),
	}
}

// Available reports whether tracks can be listed on this computer.
func (s *Service) Available() bool {
	return s.prober.Available()
}

// Streams lists the audio and subtitle tracks of a video. Anything that is not a
// video, and every video when ffprobe is missing, has none: the person then
// simply gets no choice, which is how things were before. The errors are
// media.ErrNotFound for an unknown ID, and whatever the probe reports for a file
// it cannot read.
func (s *Service) Streams(ctx context.Context, id media.ID) (app.Streams, error) {
	it, err := s.items.Get(id)
	if err != nil {
		return app.Streams{}, err
	}
	return s.streamsOf(ctx, it)
}

func (s *Service) streamsOf(ctx context.Context, it media.Item) (app.Streams, error) {
	if it.Kind != media.KindVideo || !s.prober.Available() {
		return app.Streams{}, nil
	}
	k := seenKey{it.ID, it.ModTime, it.Size}
	s.mu.Lock()
	got, ok := s.seen[k]
	s.mu.Unlock()
	if ok {
		return got, nil
	}

	got, err := s.prober.Probe(ctx, app.StreamProbe{Root: it.Root, Path: it.Path})
	if err != nil {
		return app.Streams{}, fmt.Errorf("look inside %q: %w", it.Name, err)
	}
	s.mu.Lock()
	if len(s.seen) >= maxRemembered {
		clear(s.seen)
	}
	s.seen[k] = got
	s.mu.Unlock()
	return got, nil
}

// Subtitle returns one embedded subtitle track as WebVTT. index is the track's
// position in Streams(...).Subtitles. The errors are media.ErrNotFound,
// app.ErrNoSubtitleTrack for a position that is not there or a track made of
// pictures, and app.ErrSubtitlesUnavailable when ffmpeg is missing.
//
// Requests for the same track at the same time share one extraction, and a
// finished one is kept on disk: finding a track means reading the whole video.
func (s *Service) Subtitle(ctx context.Context, id media.ID, index int) ([]byte, error) {
	it, err := s.items.Get(id)
	if err != nil {
		return nil, err
	}
	if !s.prober.Available() {
		return nil, app.ErrSubtitlesUnavailable
	}
	streams, err := s.streamsOf(ctx, it)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(streams.Subtitles) || !streams.Subtitles[index].Text {
		return nil, app.ErrNoSubtitleTrack
	}

	key := cacheKey(it, index)
	if data, ok := s.cache.Get(key); ok {
		return data, nil
	}
	j := s.attempt(key, it, index)
	select {
	case <-j.done:
		return j.data, j.err
	case <-ctx.Done():
		// This caller gave up; the extraction carries on for the others, and for
		// the cache.
		return nil, ctx.Err()
	}
}

// attempt returns the running extraction for key, starting one if there is none.
func (s *Service) attempt(key string, it media.Item, index int) *job {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.inflight[key]; ok {
		return j
	}
	j := &job{done: make(chan struct{})}
	s.inflight[key] = j
	// It ends when ffmpeg finishes or its own timeout kills it; nobody has to
	// stop it.
	go s.run(key, it, index, j)
	return j
}

func (s *Service) run(key string, it media.Item, index int, j *job) {
	s.slots <- struct{}{}
	defer func() { <-s.slots }()

	// Not the requester's context: it may be cancelled while others still wait.
	data, err := s.extract.Extract(context.Background(), app.SubtitleExtraction{Root: it.Root, Path: it.Path, Index: index})
	if err == nil {
		if putErr := s.cache.Put(key, data); putErr != nil {
			s.log.Warn("could not remember a subtitle", slog.String("error", putErr.Error()))
		}
	} else {
		s.log.Debug("no subtitle", slog.String("name", it.Name), slog.Int("track", index), slog.String("reason", err.Error()))
	}

	s.mu.Lock()
	delete(s.inflight, key)
	s.mu.Unlock()

	j.data, j.err = data, err
	close(j.done)
}

// cacheKey names an extracted track by the video, the exact file it was read
// from and the track, so an edited file gets new subtitles. Every part is safe
// in a file name.
func cacheKey(it media.Item, index int) string {
	return fmt.Sprintf("%s-e%d-%d-%d-v%d", it.ID, index, it.Size, it.ModTime.UnixNano(), version)
}
