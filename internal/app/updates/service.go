// Package updates is the use case behind "is there a newer EinCast?": ask the
// release source now and then, compare with the running version, and tell the
// person. It never downloads or installs anything (ADR 0029).
package updates

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/version"
)

// devBuildMessage is shown when the running build has no release number, such as
// one made from source: there is nothing to compare, and nothing is asked.
const devBuildMessage = "This build was made from source and has no version number, so there is nothing to compare with."

// Settings is the part of the settings this service reads. Declared here, at
// the consumer.
type Settings interface {
	Get() app.SettingsView
}

// Timing says when automatic checks happen.
type Timing struct {
	// FirstAfter is the wait after start-up before the first look, so the window
	// and the library come first.
	FirstAfter time.Duration
	// PollEvery is how often the service wakes to see whether a check is due. A
	// wake costs nothing: it asks the network only when a check is due.
	PollEvery time.Duration
	// MaxAge is how old the last successful check may be before an automatic one
	// is due. A failed check is retried at the next wake.
	MaxAge time.Duration
}

// DefaultTiming checks shortly after start-up and then about once a day. Waking
// every hour (and not once a day) means turning the setting on takes effect
// within the hour, and a computer that slept through the day catches up.
var DefaultTiming = Timing{FirstAfter: 15 * time.Second, PollEvery: time.Hour, MaxAge: 24 * time.Hour}

// Service owns the update status.
type Service struct {
	source   app.ReleaseSource
	settings Settings
	events   app.EventPublisher
	log      *slog.Logger
	timing   Timing
	platform string
	now      func() time.Time // a field so tests control time

	current    string
	currentVer version.Version
	comparable bool // current parsed as a version

	mu        sync.Mutex
	checking  bool
	latest    *app.UpdateRelease
	available bool
	checkedAt time.Time
	errMsg    string
}

// Option customizes a Service.
type Option func(*Service)

// WithPlatform sets the operating system ("darwin", "windows") whose files the
// notice lists. Without it, or for an unknown system, every file is listed.
func WithPlatform(goos string) Option {
	return func(s *Service) { s.platform = goos }
}

// NewService builds a Service. current is the running version as stamped at
// build time ("1.1.1", or "dev").
func NewService(source app.ReleaseSource, settings Settings, events app.EventPublisher, log *slog.Logger,
	current string, timing Timing, opts ...Option) *Service {
	s := &Service{
		source:   source,
		settings: settings,
		events:   events,
		log:      log.With(slog.String("source", "updates")),
		timing:   timing,
		now:      time.Now,
		current:  current,
	}
	for _, opt := range opts {
		opt(s)
	}
	if v, err := version.Parse(current); err == nil {
		s.currentVer, s.comparable = v, true
	}
	return s
}

// Status returns what is known now.
func (s *Service) Status() app.UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

// PageURL returns the address of the newest release's page, or "" when there is
// no newer version or its address is not a plain https one. It is what the
// "Open download page" button opens: the caller supplies no address, so the
// button cannot be made to open anything else.
func (s *Service) PageURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available || s.latest == nil {
		return ""
	}
	return s.latest.URL
}

// Check asks the release source now, whatever the setting says: the person
// pressed the button. It returns the status afterwards; a failure is in
// Status.Error, not a returned error, because the UI shows it as a state. While
// another check is running it returns the status without asking again.
func (s *Service) Check(ctx context.Context) app.UpdateStatus {
	if !s.comparable {
		s.mu.Lock()
		s.errMsg = devBuildMessage
		st := s.statusLocked()
		s.mu.Unlock()
		s.events.Publish(app.UpdateChanged{Status: st})
		return st
	}

	s.mu.Lock()
	if s.checking {
		st := s.statusLocked()
		s.mu.Unlock()
		return st
	}
	s.checking = true
	st := s.statusLocked()
	s.mu.Unlock()
	s.events.Publish(app.UpdateChanged{Status: st})

	rel, err := s.source.Latest(ctx)
	st = s.finish(rel, err)
	s.events.Publish(app.UpdateChanged{Status: st})
	return st
}

// finish records the outcome of a check.
func (s *Service) finish(rel app.Release, err error) app.UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checking = false

	if err != nil {
		s.errMsg = failureMessage(err)
		s.log.Info("could not check for a newer version", slog.String("error", err.Error()))
		return s.statusLocked()
	}
	latest, err := version.Parse(rel.Tag)
	if err != nil {
		s.errMsg = "The newest release has an unexpected name (" + rel.Tag + ")."
		s.log.Warn("release tag is not a version", slog.String("tag", rel.Tag))
		return s.statusLocked()
	}

	s.errMsg = ""
	s.checkedAt = s.now()
	s.available = latest.Newer(s.currentVer)
	s.latest = &app.UpdateRelease{
		Version:     latest.String(),
		Name:        rel.Name,
		Notes:       rel.Notes,
		URL:         httpsOnly(rel.URL),
		PublishedAt: rel.PublishedAt,
		Assets:      assetsFor(s.platform, rel.Assets),
	}
	s.log.Info("checked for a newer version",
		slog.String("running", s.current), slog.String("newest", latest.String()), slog.Bool("available", s.available))
	return s.statusLocked()
}

func (s *Service) statusLocked() app.UpdateStatus {
	st := app.UpdateStatus{
		Current:   s.current,
		Checking:  s.checking,
		Available: s.available,
		CheckedAt: s.checkedAt,
		Error:     s.errMsg,
	}
	if s.latest != nil {
		l := *s.latest
		l.Assets = append([]app.ReleaseAsset(nil), l.Assets...)
		st.Latest = &l
	}
	return st
}

// Run checks by itself until ctx ends: once soon after start-up, then whenever
// the last good check is old enough. With the setting off it asks nothing. A
// build without a version number never asks. It returns when ctx ends.
func (s *Service) Run(ctx context.Context) {
	if !s.comparable {
		s.log.Info("development build; not checking for updates", slog.String("version", s.current))
		return
	}
	timer := time.NewTimer(s.timing.FirstAfter)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.checkIfDue(ctx)
			timer.Reset(s.timing.PollEvery)
		}
	}
}

// checkIfDue is one wake of Run.
func (s *Service) checkIfDue(ctx context.Context) {
	if !s.settings.Get().CheckForUpdates {
		return
	}
	s.mu.Lock()
	fresh := !s.checkedAt.IsZero() && s.now().Sub(s.checkedAt) < s.timing.MaxAge
	s.mu.Unlock()
	if fresh {
		return
	}
	s.Check(ctx)
}

// failureMessage turns a failed check into words for the person.
func failureMessage(err error) string {
	switch {
	case errors.Is(err, app.ErrNoRelease):
		return "No release has been published yet."
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "GitHub did not answer in time. Check your internet connection and try again."
	default:
		return "Could not check for updates: " + err.Error()
	}
}

// httpsOnly returns raw when it is a plain https address and "" otherwise. The
// address is opened in the person's browser, so anything else (a file: or custom
// scheme) must never get that far.
func httpsOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return raw
}

// assetsFor keeps the files made for goos. The release workflow names them
// eincast-<version>-macos-... and eincast-<version>-windows-...
func assetsFor(goos string, all []app.ReleaseAsset) []app.ReleaseAsset {
	marker := map[string]string{"darwin": "macos", "windows": "windows"}[goos]
	if marker == "" {
		return all
	}
	var out []app.ReleaseAsset
	for _, a := range all {
		if strings.Contains(strings.ToLower(a.Name), marker) {
			out = append(out, a)
		}
	}
	return out
}
