package ffmpeg

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
)

// Option customizes a Grabber or a Transcoder.
type Option func(*finder)

// WithChosenPath makes the program use the ffmpeg the person pointed at, when
// there is one. chosen is asked every time, so a change in Settings applies
// without a restart. An empty answer means "look for it yourself".
func WithChosenPath(chosen func() string) Option {
	return func(f *finder) { f.chosen = chosen }
}

// WithHardwareEncoding lets conversions use the computer's video chip when
// ffmpeg can. on is asked every time, so a change in Settings applies to the
// next conversion without a restart. Without this option only the processor is
// used.
func WithHardwareEncoding(on func() bool) Option {
	return func(f *finder) { f.hardwareOn = on }
}

// finder locates the ffmpeg program and remembers where it was found.
type finder struct {
	log *slog.Logger
	// find locates the program; a field so tests can point at a stand-in.
	find func() (string, error)
	// chosen returns the location the person set, or "". Nil when there is no
	// such setting.
	chosen func() string
	// hardwareOn says whether to use the video chip; nil means no.
	hardwareOn func() bool

	mu   sync.Mutex
	path string // the program, once found by searching
}

// init sets f up in place: a finder holds a mutex, so it is never copied.
func (f *finder) init(log *slog.Logger, opts []Option) {
	f.log, f.find = log, locate
	for _, opt := range opts {
		opt(f)
	}
}

// binary returns the ffmpeg location. A location the person chose wins, and if
// it stops working that is an error to show them, never a silent switch to
// some other ffmpeg. Otherwise it searches again each time until it is found, so
// installing ffmpeg while EinCast runs takes effect without a restart.
func (f *finder) binary() (string, error) {
	if f.chosen != nil {
		if p := f.chosen(); p != "" {
			if err := CheckExecutable(p); err != nil {
				return "", fmt.Errorf("the ffmpeg you chose: %w", err)
			}
			return p, nil
		}
	}

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

// Path returns where ffmpeg is, or "" when it cannot be found.
func (f *finder) Path() string {
	p, err := f.binary()
	if err != nil {
		return ""
	}
	return p
}

// Check reports why path cannot be used as ffmpeg, in words for the person. It
// is a method so the bridge can hold it as an interface.
func (*finder) Check(path string) error { return CheckExecutable(path) }

// CheckExecutable reports why path cannot be run, in words for the person.
func CheckExecutable(path string) error {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("there is no file at %s", path)
	case err != nil:
		return fmt.Errorf("cannot look at %s: %w", path, err)
	case info.IsDir():
		return fmt.Errorf("%s is a folder, not the program", path)
	case runtime.GOOS != "windows" && info.Mode()&0o111 == 0:
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}
