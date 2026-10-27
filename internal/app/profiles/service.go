// Package profiles holds the use cases behind "who is watching": the profiles and
// their PINs, the credentials that prove a device chose one, and the one question
// every request asks, "may this profile see that item?" (ADR 0027).
package profiles

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
)

// secretKey is where the credential-signing secret lives in the SecretStore.
const secretKey = "profile-secret"

// MaxProfiles is how many profiles a household can have. A picker of more than
// this is not a picker.
const MaxProfiles = 12

const (
	// pinRounds is PBKDF2's work factor. A PIN has at most a million values, so
	// this only slows a casual attempt; the lockout below is what stops guessing.
	pinRounds = 100_000

	// Wrong PINs: this many in a row from one client for one profile lock that
	// pair out for lockFor.
	maxFailures = 5
	lockFor     = 5 * time.Minute
	// maxTracked bounds the failure table, so a flood of clients cannot grow it.
	maxTracked = 4096
)

var (
	// ErrUnknownProfile means no profile has that ID.
	ErrUnknownProfile = errors.New("no such profile")
	// ErrWrongPIN means the PIN did not match.
	ErrWrongPIN = errors.New("wrong PIN")
	// ErrLastProfile means the only profile cannot be deleted: there must always be
	// someone to be.
	ErrLastProfile = errors.New("at least one profile must remain")
	// ErrTooMany means the household is at MaxProfiles.
	ErrTooMany = errors.New("there is a limit on the number of profiles")
)

// Libraries is what the service needs to know about the libraries: which exist and
// which are kept off TVs. Declared here, at the consumer.
type Libraries interface {
	Libraries() []media.Library
}

// Summary is what a device needs to draw the picker: no restrictions, no PIN.
type Summary struct {
	ID     profile.ID
	Name   string
	Color  string
	HasPIN bool
}

// Option customizes a Service.
type Option func(*Service)

// WithOnDelete runs f after a profile is deleted, so what belongs to it (its
// resume positions) goes too.
func WithOnDelete(f func(profile.ID)) Option { return func(s *Service) { s.onDelete = f } }

// Service owns the profiles. It is safe for concurrent use: requests ask it all
// the time while the desktop app edits.
type Service struct {
	store    app.ProfileStore
	secrets  app.SecretStore
	libs     Libraries
	events   app.EventPublisher
	log      *slog.Logger
	onDelete func(profile.ID)

	// now and random are replaceable so tests are exact, and rounds so they are fast.
	now    func() time.Time
	random func(n int) ([]byte, error)
	rounds int

	mu     sync.RWMutex
	recs   []app.ProfileRecord
	secret []byte

	fmu      sync.Mutex // guards failures, apart from mu so a guesser never holds up readers
	failures map[string]*failure
}

type failure struct {
	count int
	until time.Time // locked out until; zero when not locked
}

// NewService returns a Service holding only Everyone; call Load at startup.
func NewService(store app.ProfileStore, secrets app.SecretStore, libs Libraries, events app.EventPublisher, log *slog.Logger, opts ...Option) *Service {
	s := &Service{
		store: store, secrets: secrets, libs: libs, events: events,
		log:      log.With(slog.String("source", "profiles")),
		now:      time.Now,
		random:   randomBytes,
		rounds:   pinRounds,
		recs:     []app.ProfileRecord{everyone()},
		failures: map[string]*failure{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func everyone() app.ProfileRecord {
	return app.ProfileRecord{Profile: profile.Everyone(), Version: 1}
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

// Load reads the saved profiles and the signing secret. It never fails: an install
// with nothing saved has Everyone, and an unreadable or invalid file is reported
// and replaced in memory by what can be kept, because an app that refuses to start
// over a profile file is worse than one that starts with the default.
func (s *Service) Load() {
	recs, err := s.store.Load()
	switch {
	case err == nil:
	case errors.Is(err, app.ErrNoProfiles):
		recs = nil
	default:
		s.log.Warn("could not use the saved profiles; starting with the default one", slog.String("error", err.Error()))
		recs = nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = s.sanitize(recs)
	s.loadSecretLocked()
}

// sanitize keeps the records that are valid and have a unique ID, and makes sure
// there is at least one, so a hand-edited file cannot lock everyone out.
func (s *Service) sanitize(in []app.ProfileRecord) []app.ProfileRecord {
	var out []app.ProfileRecord
	seen := map[profile.ID]bool{}
	for _, r := range in {
		r.Profile = r.Profile.Clean()
		switch {
		case r.Profile.ID == "" || seen[r.Profile.ID]:
			s.log.Warn("ignoring a saved profile with a missing or repeated id", slog.String("name", r.Profile.Name))
			continue
		case r.Profile.Validate() != nil:
			s.log.Warn("ignoring an invalid saved profile", slog.String("name", r.Profile.Name), slog.String("reason", r.Profile.Validate().Error()))
			continue
		case len(r.PINHash) != 0 && len(r.PINSalt) == 0:
			s.log.Warn("a saved profile has a PIN hash but no salt; clearing its PIN", slog.String("name", r.Profile.Name))
			r.PINHash, r.PINSalt = nil, nil
		}
		if r.Version < 1 {
			r.Version = 1
		}
		seen[r.Profile.ID] = true
		out = append(out, r)
		if len(out) == MaxProfiles {
			break
		}
	}
	if len(out) == 0 {
		return []app.ProfileRecord{everyone()}
	}
	return out
}

// loadSecretLocked reads the signing secret, creating it only when the store
// positively says there is none. If the store is merely unavailable, a secret for
// this run is used and the saved one is left alone, so a locked keychain cannot
// destroy it; devices then choose their profile again after a restart.
func (s *Service) loadSecretLocked() {
	enc, err := s.secrets.Get(secretKey)
	if err == nil && enc != "" {
		if b, derr := base64.RawStdEncoding.DecodeString(enc); derr == nil && len(b) >= 32 {
			s.secret = b
			return
		}
	}
	b, rerr := s.random(32)
	if rerr != nil {
		// No randomness means no safe secret. A nil secret makes every credential
		// fail to verify: closed, not open.
		s.log.Error("could not create a signing secret; nobody can choose a profile", slog.String("error", rerr.Error()))
		s.secret = nil
		return
	}
	s.secret = b
	if err != nil && !errors.Is(err, app.ErrSecretNotFound) {
		s.log.Warn("system credential store unavailable; profile sign-ins last until restart", slog.String("error", err.Error()))
		return
	}
	if serr := s.secrets.Set(secretKey, base64.RawStdEncoding.EncodeToString(b)); serr != nil {
		s.log.Warn("could not save the signing secret; profile sign-ins last until restart", slog.String("error", serr.Error()))
	}
}

// Active reports whether there is more than one profile. With one, nobody has to
// choose, and the picker and the cookie do not exist.
func (s *Service) Active() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.recs) > 1
}

// Summaries lists the profiles for a picker.
func (s *Service) Summaries() []Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Summary, len(s.recs))
	for i, r := range s.recs {
		out[i] = Summary{ID: r.Profile.ID, Name: r.Profile.Name, Color: r.Profile.Color, HasPIN: len(r.PINHash) > 0}
	}
	return out
}

// Entries lists the profiles in full for the desktop app to edit.
func (s *Service) Entries() []app.ProfileEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entriesLocked()
}

func (s *Service) entriesLocked() []app.ProfileEntry {
	out := make([]app.ProfileEntry, len(s.recs))
	for i, r := range s.recs {
		p := r.Profile
		p.Libraries, p.Kinds, p.HiddenFolders = slices.Clone(p.Libraries), slices.Clone(p.Kinds), slices.Clone(p.HiddenFolders)
		out[i] = app.ProfileEntry{Profile: p, HasPIN: len(r.PINHash) > 0}
	}
	return out
}

// Only returns the lone profile when there is exactly one. It is what every
// request is, without a choice to make.
func (s *Service) Only() (profile.Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.recs) != 1 {
		return profile.Profile{}, false
	}
	return s.recs[0].Profile, true
}

// Save adds a profile (an empty ID) or changes one. The PIN is not part of it; see
// SetPIN. It returns the profile as stored.
func (s *Service) Save(p profile.Profile) (profile.Profile, error) {
	p = p.Clean()
	if err := p.Validate(); err != nil {
		return profile.Profile{}, err
	}

	s.mu.Lock()
	next := slices.Clone(s.recs)
	if p.ID == "" {
		if len(next) >= MaxProfiles {
			s.mu.Unlock()
			return profile.Profile{}, ErrTooMany
		}
		id, err := s.newIDLocked()
		if err != nil {
			s.mu.Unlock()
			return profile.Profile{}, fmt.Errorf("new profile: %w", err)
		}
		p.ID = id
		next = append(next, app.ProfileRecord{Profile: p, Version: 1})
	} else {
		i := indexOf(next, p.ID)
		if i < 0 {
			s.mu.Unlock()
			return profile.Profile{}, ErrUnknownProfile
		}
		next[i].Profile = p
	}
	if err := s.commitLocked(next); err != nil {
		s.mu.Unlock()
		return profile.Profile{}, err
	}
	entries := s.entriesLocked()
	s.mu.Unlock()

	s.events.Publish(app.ProfilesChanged{Profiles: entries})
	return p, nil
}

// newIDLocked makes an ID that no profile has, now or (since they are random)
// later.
func (s *Service) newIDLocked() (profile.ID, error) {
	b, err := s.random(8)
	if err != nil {
		return "", err
	}
	id := profile.ID(hex.EncodeToString(b))
	if indexOf(s.recs, id) >= 0 {
		return "", errors.New("id collision")
	}
	return id, nil
}

// Delete removes a profile, ends every sign-in made as it, and forgets what
// belongs to it.
func (s *Service) Delete(id profile.ID) error {
	s.mu.Lock()
	i := indexOf(s.recs, id)
	if i < 0 {
		s.mu.Unlock()
		return ErrUnknownProfile
	}
	if len(s.recs) == 1 {
		s.mu.Unlock()
		return ErrLastProfile
	}
	next := slices.Delete(slices.Clone(s.recs), i, i+1)
	if err := s.commitLocked(next); err != nil {
		s.mu.Unlock()
		return err
	}
	entries := s.entriesLocked()
	s.mu.Unlock()

	if s.onDelete != nil {
		s.onDelete(id)
	}
	s.events.Publish(app.ProfilesChanged{Profiles: entries})
	return nil
}

// SetPIN sets or changes a profile's PIN. Changing it signs out every device that
// chose the profile with the old one.
func (s *Service) SetPIN(id profile.ID, pin string) error {
	if err := profile.ValidatePIN(pin); err != nil {
		return err
	}
	salt, err := s.random(16)
	if err != nil {
		return fmt.Errorf("set PIN: %w", err)
	}
	hash, err := s.hashPIN(pin, salt)
	if err != nil {
		return fmt.Errorf("set PIN: %w", err)
	}
	return s.change(id, func(r *app.ProfileRecord) {
		r.PINHash, r.PINSalt = hash, salt
		r.Version++
	})
}

// ClearPIN removes a profile's PIN. Devices that chose it are signed out too, so a
// credential never outlives the rule it was issued under.
func (s *Service) ClearPIN(id profile.ID) error {
	return s.change(id, func(r *app.ProfileRecord) {
		r.PINHash, r.PINSalt = nil, nil
		r.Version++
	})
}

func (s *Service) change(id profile.ID, edit func(*app.ProfileRecord)) error {
	s.mu.Lock()
	next := slices.Clone(s.recs)
	i := indexOf(next, id)
	if i < 0 {
		s.mu.Unlock()
		return ErrUnknownProfile
	}
	edit(&next[i])
	if err := s.commitLocked(next); err != nil {
		s.mu.Unlock()
		return err
	}
	entries := s.entriesLocked()
	s.mu.Unlock()
	s.events.Publish(app.ProfilesChanged{Profiles: entries})
	return nil
}

// commitLocked saves next and, only if that worked, makes it current. A failed
// save changes nothing.
func (s *Service) commitLocked(next []app.ProfileRecord) error {
	if err := s.store.Save(next); err != nil {
		return fmt.Errorf("save profiles: %w", err)
	}
	s.recs = next
	return nil
}

func indexOf(recs []app.ProfileRecord, id profile.ID) int {
	return slices.IndexFunc(recs, func(r app.ProfileRecord) bool { return r.Profile.ID == id })
}

func (s *Service) hashPIN(pin string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, pin, salt, s.rounds, 32)
}

// Select checks that client may be profile id and, if so, returns the credential
// that proves it. A profile with no PIN needs none. A wrong PIN counts against the
// client and profile together; at the limit they are locked out for a while, and
// the answer is the same whether or not the PIN would have been right.
func (s *Service) Select(client string, id profile.ID, pin string) (string, profile.Profile, error) {
	s.mu.RLock()
	i := indexOf(s.recs, id)
	if i < 0 {
		s.mu.RUnlock()
		return "", profile.Profile{}, ErrUnknownProfile
	}
	rec := s.recs[i]
	cred, signed := s.credentialLocked(rec)
	s.mu.RUnlock()

	if len(rec.PINHash) > 0 {
		key := client + "|" + string(id)
		if wait := s.locked(key); wait > 0 {
			return "", profile.Profile{}, &app.LockedError{RetryAfter: wait}
		}
		// Hashing takes a moment on purpose, so it runs with no lock held: a person
		// guessing must not make the rest of the household wait.
		got, err := s.hashPIN(pin, rec.PINSalt)
		if err != nil || subtle.ConstantTimeCompare(got, rec.PINHash) != 1 {
			s.fail(key)
			return "", profile.Profile{}, ErrWrongPIN
		}
		s.forgive(key)
	}
	if !signed {
		return "", profile.Profile{}, errors.New("profile sign-in is unavailable")
	}
	return cred, rec.Profile, nil
}

func (s *Service) locked(key string) time.Duration {
	s.fmu.Lock()
	defer s.fmu.Unlock()
	f := s.failures[key]
	if f == nil || f.until.IsZero() {
		return 0
	}
	if wait := f.until.Sub(s.now()); wait > 0 {
		return wait
	}
	delete(s.failures, key) // the lock ran out
	return 0
}

func (s *Service) fail(key string) {
	s.fmu.Lock()
	defer s.fmu.Unlock()
	if len(s.failures) >= maxTracked {
		s.pruneLocked()
	}
	f := s.failures[key]
	if f == nil {
		f = &failure{}
		s.failures[key] = f
	}
	f.count++
	if f.count >= maxFailures {
		f.until = s.now().Add(lockFor)
		f.count = 0
	}
}

func (s *Service) forgive(key string) {
	s.fmu.Lock()
	defer s.fmu.Unlock()
	delete(s.failures, key)
}

// pruneLocked drops expired locks, and everything if that frees nothing: a flood
// of distinct clients must not grow the table without bound, and forgetting
// partial counts only gives a guesser a few more tries per client.
func (s *Service) pruneLocked() {
	for k, f := range s.failures {
		if !f.until.IsZero() && !f.until.After(s.now()) {
			delete(s.failures, k)
		}
	}
	if len(s.failures) >= maxTracked {
		clear(s.failures)
	}
}

// credentialLocked signs "id.version" for a profile.
func (s *Service) credentialLocked(r app.ProfileRecord) (string, bool) {
	if len(s.secret) == 0 {
		return "", false
	}
	body := string(r.Profile.ID) + "." + strconv.Itoa(r.Version)
	return body + "." + s.mac(body), true
}

func (s *Service) mac(body string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(body))
	return hex.EncodeToString(m.Sum(nil))
}

// Credential returns the credential for a profile that a request is already
// authenticated as, to put in a playlist for a player that cannot keep a cookie.
func (s *Service) Credential(id profile.ID) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i := indexOf(s.recs, id)
	if i < 0 {
		return "", false
	}
	return s.credentialLocked(s.recs[i])
}

// Resolve returns the profile a credential names, if it is genuine and still
// current: signed with this secret, for a profile that exists, under the PIN
// version it was issued for.
func (s *Service) Resolve(credential string) (profile.Profile, bool) {
	parts := strings.Split(credential, ".")
	if len(parts) != 3 {
		return profile.Profile{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.secret) == 0 {
		return profile.Profile{}, false
	}
	want := s.mac(parts[0] + "." + parts[1])
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(want)) != 1 {
		return profile.Profile{}, false
	}
	i := indexOf(s.recs, profile.ID(parts[0]))
	if i < 0 || strconv.Itoa(s.recs[i].Version) != parts[1] {
		return profile.Profile{}, false
	}
	return s.recs[i].Profile, true
}

// Visible returns the items p may see, in order. The slice is new.
func Visible(p profile.Profile, items []media.Item) []media.Item {
	out := make([]media.Item, 0, len(items))
	for _, it := range items {
		if p.Allows(it) {
			out = append(out, it)
		}
	}
	return out
}

// TVVisible reports whether a smart TV may see the item: its library is not kept
// off TVs. A TV cannot say who is watching, so this is the only rule it gets.
func (s *Service) TVVisible(it media.Item) bool {
	for _, l := range s.libs.Libraries() {
		if l.ID == it.LibraryID {
			return !l.HiddenFromTVs
		}
	}
	return false // a library that is gone shows nothing
}
