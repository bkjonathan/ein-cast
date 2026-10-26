// Package thumbnails is the use case behind preview images: look the item up,
// serve a remembered thumbnail if there is one, otherwise make it once.
package thumbnails

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// Width is how wide thumbnails are made, in pixels: sharp on a high-density
// screen at the size the apps show them, still only a few tens of kilobytes.
const Width = 320

// version is part of every cache key. Raise it when thumbnails should look
// different (size, quality), so old ones are made again.
const version = 1

// Items is the part of the library this service reads. Declared here, at the
// consumer.
type Items interface {
	Get(id media.ID) (media.Item, error)
}

// Thumbnail is a made preview image.
type Thumbnail struct {
	// JPEG is the image.
	JPEG []byte
	// ModTime is when the source file last changed, for HTTP caching.
	ModTime time.Time
}

// Options tune a Service.
type Options struct {
	// Parallel is how many thumbnails are made at once. Each one is a separate
	// ffmpeg process reading a possibly large file, so a few is plenty.
	Parallel int
	// Timeout caps one attempt, so a file that makes ffmpeg hang cannot hold a
	// slot forever.
	Timeout time.Duration
}

// DefaultOptions suit a home computer.
var DefaultOptions = Options{Parallel: 2, Timeout: 30 * time.Second}

// Service makes thumbnails on demand. Nothing is made ahead of time: opening
// the library costs nothing, and only what is looked at is generated.
type Service struct {
	items Items
	grab  app.FrameGrabber
	cache app.ThumbnailCache
	log   *slog.Logger
	opts  Options
	slots chan struct{} // a token per running attempt

	mu       sync.Mutex
	inflight map[string]*job
	failed   map[string]struct{} // keys that did not work this run; not retried
}

// job is one attempt that several requests may wait on.
type job struct {
	done chan struct{} // closed once data and err are set
	data []byte
	err  error
}

// NewService builds a Service from its ports.
func NewService(items Items, grab app.FrameGrabber, cache app.ThumbnailCache, log *slog.Logger, opts Options) *Service {
	opts.Parallel = max(opts.Parallel, 1)
	return &Service{
		items:    items,
		grab:     grab,
		cache:    cache,
		log:      log.With(slog.String("source", "thumbnails")),
		opts:     opts,
		slots:    make(chan struct{}, opts.Parallel),
		inflight: make(map[string]*job),
		failed:   make(map[string]struct{}),
	}
}

// Available reports whether new thumbnails can be made on this computer. Ones
// made before are served either way.
func (s *Service) Available() bool {
	return s.grab.Available()
}

// Get returns the thumbnail of an item. The errors are media.ErrNotFound for an
// unknown ID, app.ErrThumbnailsUnavailable when ffmpeg is missing, and
// app.ErrNoThumbnail when the item has nothing to show.
//
// Requests for the same item at the same time share one attempt, and an item
// that failed is not tried again until the app restarts or the file changes.
func (s *Service) Get(ctx context.Context, id media.ID) (Thumbnail, error) {
	it, err := s.items.Get(id)
	if err != nil {
		return Thumbnail{}, err
	}
	key := cacheKey(it)
	if data, ok := s.cache.Get(key); ok {
		return Thumbnail{JPEG: data, ModTime: it.ModTime}, nil
	}
	if !s.grab.Available() {
		return Thumbnail{}, app.ErrThumbnailsUnavailable
	}

	j, err := s.attempt(key, it)
	if err != nil {
		return Thumbnail{}, err
	}
	select {
	case <-j.done:
	case <-ctx.Done():
		// This caller gave up; the attempt carries on for the others, and for
		// the cache.
		return Thumbnail{}, ctx.Err()
	}
	if j.err != nil {
		return Thumbnail{}, j.err
	}
	return Thumbnail{JPEG: j.data, ModTime: it.ModTime}, nil
}

// attempt returns the running attempt for key, starting one if there is none.
func (s *Service) attempt(key string, it media.Item) (*job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, bad := s.failed[key]; bad {
		return nil, app.ErrNoThumbnail
	}
	if j, ok := s.inflight[key]; ok {
		return j, nil
	}
	j := &job{done: make(chan struct{})}
	s.inflight[key] = j
	// It ends when ffmpeg finishes or the timeout kills it; nobody has to stop it.
	go s.run(key, it, j)
	return j, nil
}

func (s *Service) run(key string, it media.Item, j *job) {
	s.slots <- struct{}{}
	defer func() { <-s.slots }()

	// Not the requester's context: it may be cancelled while others still wait.
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.Timeout)
	defer cancel()

	data, err := s.grab.Grab(ctx, app.GrabRequest{Root: it.Root, Path: it.Path, Kind: it.Kind, Width: Width})
	if err == nil {
		if putErr := s.cache.Put(key, data); putErr != nil {
			s.log.Warn("could not remember a thumbnail", slog.String("error", putErr.Error()))
		}
	} else {
		s.log.Debug("no thumbnail", slog.String("name", it.Name), slog.String("reason", err.Error()))
	}

	s.mu.Lock()
	delete(s.inflight, key)
	if err != nil && !errors.Is(err, app.ErrThumbnailsUnavailable) {
		s.failed[key] = struct{}{}
		if !errors.Is(err, app.ErrNoThumbnail) {
			err = fmt.Errorf("%w: %v", app.ErrNoThumbnail, err)
		}
	}
	s.mu.Unlock()

	j.data, j.err = data, err
	close(j.done)
}

// cacheKey names a thumbnail by the item and the exact file it was made from,
// so an edited or replaced file gets a new thumbnail. Every part is safe in a
// file name.
func cacheKey(it media.Item) string {
	return fmt.Sprintf("%s-%d-%d-v%d", it.ID, it.Size, it.ModTime.UnixNano(), version)
}
