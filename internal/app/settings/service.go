// Package settings holds the use cases for user preferences and the access
// token: loading them at startup, validating and saving changes, and answering
// "may this request in?".
package settings

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"eincast/internal/app"
	"eincast/internal/app/streaming"
	"eincast/internal/domain/media"
)

// tokenKey is where the access token lives in the SecretStore.
const tokenKey = "access-token"

// Ports below 1024 need elevated privileges on most systems, so are not offered.
const minPort, maxPort = 1024, 65535

// ErrDLNAWithToken is returned when asked to turn on Smart TV discovery together
// with the access token. A TV cannot present the token, so the two cannot both
// be on: the user's choice is explained rather than silently resolved.
var ErrDLNAWithToken = errors.New("smart TV discovery can't be on while the access token is: TVs can't send the token, so turn one of them off")

// ErrTokenOff is returned when asked to rotate a token that is switched off.
var ErrTokenOff = errors.New("access token is turned off")

// Default returns the settings used when a saved file cannot be used. It keeps
// the choices that existed before they became settings: nothing starts by itself
// and the token is off. The video chip is used for conversions: it is safe to
// leave on, because a conversion that the chip cannot do falls back to the
// processor by itself. The check for a newer version is on too (ADR 0029): an
// install that never hears about a fix keeps its bugs.
func Default() app.Settings {
	return app.Settings{Port: streaming.DefaultPort, Name: streaming.DefaultName, KeepAwake: true, HardwareEncoding: true, CheckForUpdates: true}
}

// firstRun returns the settings of a new install: the same, with the protections
// and conveniences on (ADR 0024). Existing installs never pass through here, so
// an upgrade changes nothing they chose.
func (s *Service) firstRun() app.Settings {
	d := Default()
	d.RequireToken = true
	d.AutoStart = true
	d.HideOnClose = s.hideOnClose
	return d
}

// Input is the part of the settings the user edits. The library folder is not
// here: it changes only by choosing a folder.
type Input struct {
	Port         int
	Name         string
	RequireToken bool
	DLNA         bool
	AutoStart    bool
	KeepAwake    bool
	OpenAtLogin  bool
	HideOnClose  bool
	// HardwareEncoding lets conversions use the computer's video chip.
	HardwareEncoding bool
	// CheckForUpdates lets EinCast look for a newer release by itself.
	CheckForUpdates bool
}

// Service owns the settings and the access token. It implements
// app.LibraryMemory and answers the HTTP layer's Required/Verify questions.
type Service struct {
	store   app.SettingsStore
	secrets app.SecretStore
	events  app.EventPublisher
	log     *slog.Logger

	// newToken is replaceable so tests can use known tokens.
	newToken func() (string, error)

	loginItem   app.LoginItem // nil: the platform cannot start apps at login
	hideOnClose bool          // the first-run value of HideOnClose

	mu        sync.RWMutex
	cur       app.Settings
	token     string
	persisted bool
}

var _ app.LibraryMemory = (*Service)(nil)

// Option customizes a Service.
type Option func(*Service)

// WithLoginItem lets the "open at login" setting act on the computer.
func WithLoginItem(li app.LoginItem) Option {
	return func(s *Service) { s.loginItem = li }
}

// WithHideOnCloseByDefault sets whether a new install keeps sharing when its
// window is closed. It is a choice of the platform (a Dock icon or a tray icon
// has to exist to bring the window back), so main makes it.
func WithHideOnCloseByDefault(on bool) Option {
	return func(s *Service) { s.hideOnClose = on }
}

// NewService returns a Service holding the defaults; call Load at startup.
func NewService(store app.SettingsStore, secrets app.SecretStore, events app.EventPublisher, log *slog.Logger, opts ...Option) *Service {
	s := &Service{
		store:    store,
		secrets:  secrets,
		events:   events,
		log:      log.With(slog.String("source", "settings")),
		newToken: generateToken,
		cur:      Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Load reads the saved settings. It never fails: a missing file means first
// run, and an unreadable or invalid one falls back to defaults field by field,
// with a warning, because an app that refuses to start over a bad settings file
// is worse than one that starts with defaults.
func (s *Service) Load() {
	loaded, err := s.store.Load()
	switch {
	case err == nil:
	case errors.Is(err, app.ErrNoSettings):
		loaded = s.firstRun()
	default:
		s.log.Warn("could not use saved settings; starting with defaults", slog.String("error", err.Error()))
		loaded = Default()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = s.sanitize(loaded)
	if s.cur.RequireToken {
		s.loadTokenLocked()
	}
	if s.cur.OpenAtLogin && s.loginItem != nil {
		// Written again so the entry follows the app if it was moved or updated.
		if err := s.loginItem.Set(true); err != nil {
			s.log.Warn("could not refresh the open-at-login entry", slog.String("error", err.Error()))
		}
	}
}

// sanitize replaces any invalid field with its default, so a hand-edited file
// cannot make the server start on port 70000.
func (s *Service) sanitize(in app.Settings) app.Settings {
	def := Default()
	if err := validate(in.Port, in.Name); err != nil {
		s.log.Warn("saved settings are invalid; resetting the bad fields", slog.String("error", err.Error()))
		if validatePort(in.Port) != nil {
			in.Port = def.Port
		}
		if validateName(in.Name) != nil {
			in.Name = def.Name
		}
	}
	if in.DLNA && in.RequireToken {
		// A hand-edited file asking for both: keep the protection, drop the TV access.
		s.log.Warn("saved settings turn on both the access token and Smart TV discovery; keeping the token only")
		in.DLNA = false
	}
	return in
}

// Get returns the settings and token as the UI should show them.
func (s *Service) Get() app.SettingsView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.viewLocked()
}

// Update validates and saves the user's changes. Nothing changes if validation
// or saving fails.
func (s *Service) Update(in Input) (app.SettingsView, error) {
	in.Name = trim(in.Name)
	if err := validate(in.Port, in.Name); err != nil {
		return app.SettingsView{}, err
	}
	if in.DLNA && in.RequireToken {
		return app.SettingsView{}, ErrDLNAWithToken
	}

	s.mu.Lock()
	next := s.cur
	next.Port, next.Name, next.RequireToken, next.DLNA = in.Port, in.Name, in.RequireToken, in.DLNA
	next.AutoStart, next.KeepAwake, next.OpenAtLogin, next.HideOnClose = in.AutoStart, in.KeepAwake, in.OpenAtLogin, in.HideOnClose
	next.HardwareEncoding, next.CheckForUpdates = in.HardwareEncoding, in.CheckForUpdates
	if err := s.applyLoginItemLocked(next.OpenAtLogin); err != nil {
		s.mu.Unlock()
		return app.SettingsView{}, err
	}
	if err := s.store.Save(next); err != nil {
		s.undoLoginItemLocked(next.OpenAtLogin)
		s.mu.Unlock()
		return app.SettingsView{}, fmt.Errorf("save settings: %w", err)
	}
	wasRequired := s.cur.RequireToken
	s.cur = next

	switch {
	case next.RequireToken && !wasRequired:
		s.token = "" // always start a fresh token when switching on
		s.provisionTokenLocked()
	case !next.RequireToken && wasRequired:
		s.dropTokenLocked()
	}
	view := s.viewLocked()
	s.mu.Unlock()

	s.events.Publish(app.SettingsChanged{View: view})
	return view, nil
}

// applyLoginItemLocked makes the computer's login entry match want. Nothing is
// asked of the system when it already matches what was saved.
func (s *Service) applyLoginItemLocked(want bool) error {
	if s.loginItem == nil || want == s.cur.OpenAtLogin {
		return nil
	}
	if err := s.loginItem.Set(want); err != nil {
		return fmt.Errorf("open at login: %w", err)
	}
	return nil
}

// undoLoginItemLocked puts the login entry back after a save that failed, so the
// computer and the file do not disagree.
func (s *Service) undoLoginItemLocked(changedTo bool) {
	if s.loginItem == nil || changedTo == s.cur.OpenAtLogin {
		return
	}
	if err := s.loginItem.Set(s.cur.OpenAtLogin); err != nil {
		s.log.Warn("could not restore the open-at-login entry", slog.String("error", err.Error()))
	}
}

// SetFFmpegPath remembers where the person said ffmpeg is. An empty path goes
// back to looking for it. Whether the file is usable is the caller's check: this
// layer cannot look at the disk.
func (s *Service) SetFFmpegPath(path string) error {
	s.mu.Lock()
	next := s.cur
	next.FFmpegPath = path
	if err := s.store.Save(next); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("save settings: %w", err)
	}
	s.cur = next
	view := s.viewLocked()
	s.mu.Unlock()

	s.events.Publish(app.SettingsChanged{View: view})
	return nil
}

// RotateToken replaces the access token. Devices holding the old one are
// locked out until they use the new link.
func (s *Service) RotateToken() (app.SettingsView, error) {
	s.mu.Lock()
	if !s.cur.RequireToken {
		s.mu.Unlock()
		return app.SettingsView{}, ErrTokenOff
	}
	s.provisionTokenLocked()
	view := s.viewLocked()
	s.mu.Unlock()

	s.log.Info("access token rotated")
	s.events.Publish(app.SettingsChanged{View: view})
	return view, nil
}

// Libraries returns the remembered library folders in the order they were added.
// It implements app.LibraryMemory.
func (s *Service) Libraries() []media.Library {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cur.Libraries)
}

// SaveLibraries replaces the remembered library folders. It implements
// app.LibraryMemory.
func (s *Service) SaveLibraries(libs []media.Library) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	next.Libraries = slices.Clone(libs)
	if err := s.store.Save(next); err != nil {
		return fmt.Errorf("save library folders: %w", err)
	}
	s.cur = next
	return nil
}

// Required reports whether requests must carry the access token.
func (s *Service) Required() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur.RequireToken
}

// Verify reports whether candidate may access the server. With the token off,
// everything is allowed. With it on, only an exact match is, and an empty or
// missing token never is: it fails closed. The comparison takes the same time
// wherever the strings differ.
func (s *Service) Verify(candidate string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.cur.RequireToken {
		return true
	}
	if s.token == "" || candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(s.token)) == 1
}

func (s *Service) viewLocked() app.SettingsView {
	v := app.SettingsView{Settings: s.cur}
	if s.cur.RequireToken {
		v.Token, v.TokenPersisted = s.token, s.persisted
	}
	return v
}

// loadTokenLocked reads the saved token, creating one only if the store
// positively says there is none. If the store is merely unavailable (locked
// keychain), a temporary token is used and the saved one is left alone, so a
// transient failure cannot destroy a good token.
func (s *Service) loadTokenLocked() {
	tok, err := s.secrets.Get(tokenKey)
	switch {
	case err == nil && tok != "":
		s.token, s.persisted = tok, true
	case err == nil || errors.Is(err, app.ErrSecretNotFound):
		s.provisionTokenLocked()
	default:
		s.log.Warn("system credential store unavailable; using a temporary access token",
			slog.String("error", err.Error()))
		s.useTemporaryTokenLocked()
	}
}

// provisionTokenLocked creates a new token and tries to save it.
func (s *Service) provisionTokenLocked() {
	tok, err := s.newToken()
	if err != nil {
		// No randomness means no safe token. Fail closed: a token nobody knows
		// locks everyone out rather than letting everyone in.
		s.log.Error("could not generate an access token", slog.String("error", err.Error()))
		s.token, s.persisted = "", false
		return
	}
	s.token = tok
	if err := s.secrets.Set(tokenKey, tok); err != nil {
		s.log.Warn("could not save the access token to the system credential store; it will change on restart",
			slog.String("error", err.Error()))
		s.persisted = false
		return
	}
	s.persisted = true
}

func (s *Service) useTemporaryTokenLocked() {
	tok, err := s.newToken()
	if err != nil {
		s.token = ""
	} else {
		s.token = tok
	}
	s.persisted = false
}

func (s *Service) dropTokenLocked() {
	s.token, s.persisted = "", false
	if err := s.secrets.Delete(tokenKey); err != nil {
		s.log.Warn("could not remove the saved access token", slog.String("error", err.Error()))
	}
}

func generateToken() (string, error) {
	var b [24]byte // 192 bits
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
