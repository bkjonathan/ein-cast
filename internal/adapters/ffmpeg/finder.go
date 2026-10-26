package ffmpeg

import (
	"log/slog"
	"sync"
)

// finder locates the ffmpeg program and remembers where it was found.
type finder struct {
	log *slog.Logger
	// find locates the program; a field so tests can point at a stand-in.
	find func() (string, error)

	mu   sync.Mutex
	path string // the program, once found
}

// binary returns the ffmpeg location, searching again each time until it is
// found, so installing ffmpeg while EinCast runs takes effect without a restart.
func (f *finder) binary() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.path != "" {
		return f.path, nil
	}
	p, err := f.find()
	if err != nil {
		return "", err
	}
	f.path = p
	f.log.Info("found ffmpeg", slog.String("path", p))
	return p, nil
}

// Available reports whether ffmpeg can be found.
func (f *finder) Available() bool {
	_, err := f.binary()
	return err == nil
}
