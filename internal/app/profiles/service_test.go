package profiles

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
)

type fakeStore struct {
	mu      sync.Mutex
	recs    []app.ProfileRecord
	loadErr error
	saveErr error
	saves   int
}

func (f *fakeStore) Load() ([]app.ProfileRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	if f.recs == nil {
		return nil, fmt.Errorf("load: %w", app.ErrNoProfiles)
	}
	return f.recs, nil
}

func (f *fakeStore) Save(r []app.ProfileRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves++
	f.recs = append([]app.ProfileRecord(nil), r...)
	return nil
}

type fakeSecrets struct {
	m      map[string]string
	getErr error
	setErr error
}

func (f *fakeSecrets) Get(k string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.m[k]
	if !ok {
		return "", fmt.Errorf("get: %w", app.ErrSecretNotFound)
	}
	return v, nil
}
func (f *fakeSecrets) Set(k, v string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.m[k] = v
	return nil
}
func (f *fakeSecrets) Delete(k string) error { delete(f.m, k); return nil }

type fakeLibs struct{ libs []media.Library }

func (f fakeLibs) Libraries() []media.Library { return f.libs }

type fakeEvents struct {
	mu  sync.Mutex
	got [][]app.ProfileEntry
}

func (f *fakeEvents) Publish(ev app.Event) {
	if e, ok := ev.(app.ProfilesChanged); ok {
		f.mu.Lock()
		f.got = append(f.got, e.Profiles)
		f.mu.Unlock()
	}
}

type rig struct {
	svc     *Service
	store   *fakeStore
	secrets *fakeSecrets
	events  *fakeEvents
	clock   *time.Time
	deleted []profile.ID
}

func newRig(t *testing.T, libs ...media.Library) *rig {
	t.Helper()
	now := time.Unix(1_700_000_000, 0)
	r := &rig{store: &fakeStore{}, secrets: &fakeSecrets{m: map[string]string{}}, events: &fakeEvents{}, clock: &now}
	r.svc = NewService(r.store, r.secrets, fakeLibs{libs}, r.events, slog.New(slog.DiscardHandler),
		WithOnDelete(func(id profile.ID) { r.deleted = append(r.deleted, id) }))
	r.svc.now = func() time.Time { return *r.clock }
	r.svc.rounds = 1 // the real count is for people, not tests
	n := 0
	r.svc.random = func(size int) ([]byte, error) {
		n++
		return bytes.Repeat([]byte{byte(n)}, size), nil
	}
	r.svc.Load()
	return r
}

func kids() profile.Profile {
	return profile.Profile{Name: "Kids", Color: "green", Libraries: []media.LibraryID{"cartoons"}, Kinds: []media.Kind{media.KindVideo}}
}

func TestLoad_FirstRunHasEveryoneAndNoRestrictions(t *testing.T) {
	r := newRig(t)
	if r.svc.Active() {
		t.Error("one profile must not turn the picker on")
	}
	only, ok := r.svc.Only()
	if !ok || only.ID != profile.EveryoneID || !only.Allows(media.Item{LibraryID: "x", Kind: media.KindImage}) {
		t.Errorf("Only() = %+v, %v", only, ok)
	}
	if got := r.svc.Summaries(); len(got) != 1 || got[0].Name != "Everyone" || got[0].HasPIN {
		t.Errorf("Summaries() = %+v", got)
	}
}

func TestLoad_KeepsValidSavedProfilesAndDropsBrokenOnes(t *testing.T) {
	good := app.ProfileRecord{Profile: profile.Profile{ID: "a", Name: "Adults", Color: "blue", AllLibraries: true}, Version: 3}
	r := newRig(t)
	r.store.recs = []app.ProfileRecord{
		good,
		{Profile: profile.Profile{ID: "a", Name: "Repeated id", Color: "blue", AllLibraries: true}},
		{Profile: profile.Profile{ID: "", Name: "No id", Color: "blue"}},
		{Profile: profile.Profile{ID: "b", Name: "", Color: "blue"}},
		{Profile: profile.Profile{ID: "c", Name: "Bad colour", Color: "<b>"}},
		{Profile: profile.Profile{ID: "d", Name: "Hash without salt", Color: "red", AllLibraries: true}, PINHash: []byte{1}},
	}
	r.svc.Load()
	var names []string
	for _, e := range r.svc.Entries() {
		names = append(names, e.Profile.Name)
	}
	if got := strings.Join(names, ","); got != "Adults,Hash without salt" {
		t.Errorf("kept %s", got)
	}
	for _, e := range r.svc.Entries() {
		if e.Profile.Name == "Hash without salt" && e.HasPIN {
			t.Error("a hash with no salt must not count as a PIN")
		}
	}
}

func TestLoad_AnUnusableFileFallsBackToTheDefault(t *testing.T) {
	r := newRig(t)
	r.store.loadErr = errors.New("corrupt")
	r.svc.Load()
	if got := r.svc.Summaries(); len(got) != 1 || got[0].ID != profile.EveryoneID {
		t.Errorf("Summaries() = %+v, want just Everyone so nobody is locked out", got)
	}
}

func TestSave_AddsAndChanges(t *testing.T) {
	r := newRig(t)
	saved, err := r.svc.Save(kids())
	if err != nil || saved.ID == "" {
		t.Fatalf("Save() = %+v, %v", saved, err)
	}
	if !r.svc.Active() || len(r.store.recs) != 2 || len(r.events.got) != 1 {
		t.Errorf("active=%v saved=%d events=%d", r.svc.Active(), len(r.store.recs), len(r.events.got))
	}

	saved.Name = "  Little ones "
	again, err := r.svc.Save(saved)
	if err != nil || again.Name != "Little ones" || again.ID != saved.ID {
		t.Fatalf("Save() = %+v, %v", again, err)
	}
	if len(r.svc.Entries()) != 2 {
		t.Error("changing a profile added another")
	}
}

func TestSave_Refusals(t *testing.T) {
	r := newRig(t)
	if _, err := r.svc.Save(profile.Profile{Name: "", Color: "blue"}); !errors.Is(err, profile.ErrInvalid) {
		t.Errorf("empty name: %v", err)
	}
	if _, err := r.svc.Save(profile.Profile{ID: "nope", Name: "X", Color: "blue"}); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown id: %v", err)
	}
	for i := 1; i < MaxProfiles; i++ {
		if _, err := r.svc.Save(kids()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.svc.Save(kids()); !errors.Is(err, ErrTooMany) {
		t.Errorf("past the limit: %v", err)
	}
}

func TestSave_AFailedSaveChangesNothing(t *testing.T) {
	r := newRig(t)
	r.store.saveErr = errors.New("disk full")
	if _, err := r.svc.Save(kids()); err == nil {
		t.Fatal("want the error")
	}
	if r.svc.Active() || len(r.events.got) != 0 {
		t.Error("a profile that could not be saved is in use")
	}
}

func TestDelete(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())

	if err := r.svc.Delete(k.ID); err != nil {
		t.Fatal(err)
	}
	if r.svc.Active() || len(r.deleted) != 1 || r.deleted[0] != k.ID {
		t.Errorf("active=%v deleted=%v: its resume positions must go with it", r.svc.Active(), r.deleted)
	}
	if err := r.svc.Delete(profile.EveryoneID); !errors.Is(err, ErrLastProfile) {
		t.Errorf("deleting the last profile: %v", err)
	}
	if err := r.svc.Delete("nope"); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("deleting an unknown profile: %v", err)
	}
}

func TestPIN_IsStoredHashedAndSelectChecksIt(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	if err := r.svc.SetPIN(k.ID, "4321"); err != nil {
		t.Fatal(err)
	}
	for _, rec := range r.store.recs {
		if rec.Profile.ID == k.ID && (len(rec.PINHash) != 32 || len(rec.PINSalt) != 16 || bytes.Contains(rec.PINHash, []byte("4321"))) {
			t.Errorf("stored PIN = hash %x salt %x", rec.PINHash, rec.PINSalt)
		}
	}
	if !r.svc.Summaries()[1].HasPIN {
		t.Error("the picker is not told the profile has a PIN")
	}

	if _, _, err := r.svc.Select("1.2.3.4", k.ID, "0000"); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("wrong PIN: %v", err)
	}
	if _, _, err := r.svc.Select("1.2.3.4", k.ID, ""); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("no PIN: %v", err)
	}
	cred, p, err := r.svc.Select("1.2.3.4", k.ID, "4321")
	if err != nil || p.ID != k.ID || cred == "" {
		t.Fatalf("right PIN: %q %+v %v", cred, p, err)
	}
}

func TestPIN_Rules(t *testing.T) {
	r := newRig(t)
	for _, bad := range []string{"", "123", "1234567", "12ab"} {
		if err := r.svc.SetPIN(profile.EveryoneID, bad); !errors.Is(err, profile.ErrInvalid) {
			t.Errorf("SetPIN(%q) = %v", bad, err)
		}
	}
	if err := r.svc.SetPIN("nope", "1234"); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
}

func TestSelect_NoPINNeedsNone(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	cred, _, err := r.svc.Select("c", k.ID, "")
	if err != nil || cred == "" {
		t.Fatalf("Select() = %q, %v", cred, err)
	}
	if _, _, err := r.svc.Select("c", "nope", ""); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
}

func TestSelect_LocksOutAfterRepeatedWrongPINs(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	_ = r.svc.SetPIN(k.ID, "4321")

	for i := 0; i < maxFailures; i++ {
		if _, _, err := r.svc.Select("kid", k.ID, "0000"); !errors.Is(err, ErrWrongPIN) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	var locked *app.LockedError
	if _, _, err := r.svc.Select("kid", k.ID, "4321"); !errors.As(err, &locked) || locked.RetryAfter <= 0 {
		t.Fatalf("after the limit: %v, want a lock-out even for the right PIN", err)
	}
	if _, _, err := r.svc.Select("parent", k.ID, "4321"); err != nil {
		t.Errorf("another client was locked out too: %v", err)
	}

	*r.clock = r.clock.Add(lockFor + time.Second)
	if _, _, err := r.svc.Select("kid", k.ID, "4321"); err != nil {
		t.Errorf("after the lock ran out: %v", err)
	}
}

func TestSelect_ARightPINClearsTheCount(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	_ = r.svc.SetPIN(k.ID, "4321")
	for round := 0; round < 3; round++ {
		for i := 0; i < maxFailures-1; i++ {
			_, _, _ = r.svc.Select("c", k.ID, "0000")
		}
		if _, _, err := r.svc.Select("c", k.ID, "4321"); err != nil {
			t.Fatalf("round %d: %v: failures before a success must not add up", round, err)
		}
	}
}

func TestSelect_TheFailureTableIsBounded(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	_ = r.svc.SetPIN(k.ID, "4321")
	for i := 0; i < maxTracked*2; i++ {
		_, _, _ = r.svc.Select(fmt.Sprintf("client-%d", i), k.ID, "0000")
	}
	r.svc.fmu.Lock()
	n := len(r.svc.failures)
	r.svc.fmu.Unlock()
	if n > maxTracked {
		t.Errorf("tracking %d clients, want at most %d", n, maxTracked)
	}
}

func TestResolve(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	cred, _, _ := r.svc.Select("c", k.ID, "")

	p, ok := r.svc.Resolve(cred)
	if !ok || p.ID != k.ID || p.Name != "Kids" {
		t.Fatalf("Resolve() = %+v, %v", p, ok)
	}

	for name, bad := range map[string]string{
		"empty":           "",
		"no dots":         "x",
		"a forged mac":    string(k.ID) + ".1." + strings.Repeat("0", 64),
		"another profile": string(profile.EveryoneID) + ".1." + strings.Split(cred, ".")[2],
		"another version": string(k.ID) + ".2." + strings.Split(cred, ".")[2],
		"too many parts":  cred + ".x",
		"a truncated mac": cred[:len(cred)-2],
		"an unknown id":   "ghost.1." + strings.Repeat("a", 64),
	} {
		if _, ok := r.svc.Resolve(bad); ok {
			t.Errorf("%s: Resolve(%q) accepted", name, bad)
		}
	}
}

func TestResolve_ChangingOrClearingThePINEndsOldSignIns(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	cred, _, _ := r.svc.Select("c", k.ID, "")

	_ = r.svc.SetPIN(k.ID, "4321")
	if _, ok := r.svc.Resolve(cred); ok {
		t.Fatal("a sign-in made before the PIN was set still works")
	}
	cred, _, _ = r.svc.Select("c", k.ID, "4321")
	_ = r.svc.SetPIN(k.ID, "9999")
	if _, ok := r.svc.Resolve(cred); ok {
		t.Fatal("a sign-in made with the old PIN still works after it changed")
	}
	cred, _, _ = r.svc.Select("c", k.ID, "9999")
	_ = r.svc.ClearPIN(k.ID)
	if _, ok := r.svc.Resolve(cred); ok {
		t.Fatal("a sign-in survived the PIN being removed")
	}
}

func TestResolve_ADeletedProfilesCredentialDiesAndIsNotReborn(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	cred, _, _ := r.svc.Select("c", k.ID, "")
	_ = r.svc.Delete(k.ID)
	if _, ok := r.svc.Resolve(cred); ok {
		t.Fatal("a deleted profile's credential still works")
	}
	k2, _ := r.svc.Save(kids())
	if k2.ID == k.ID {
		t.Error("a new profile reused a deleted profile's id")
	}
	if _, ok := r.svc.Resolve(cred); ok {
		t.Fatal("the old credential matches a new profile")
	}
}

func TestCredentialsSurviveARestartWhenTheSecretIsSaved(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	cred, _, _ := r.svc.Select("c", k.ID, "")

	again := NewService(r.store, r.secrets, fakeLibs{}, r.events, slog.New(slog.DiscardHandler))
	again.Load()
	if _, ok := again.Resolve(cred); !ok {
		t.Error("a device had to choose its profile again after a restart")
	}
}

func TestSecret_AnUnavailableStoreDoesNotDestroyTheSavedOne(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	saved := r.secrets.m[secretKey]
	if saved == "" {
		t.Fatal("the secret was not saved")
	}

	r.secrets.getErr = errors.New("keychain locked")
	r.svc.Load()
	if r.secrets.m[secretKey] != saved {
		t.Error("a locked keychain overwrote the good secret")
	}
	if _, _, err := r.svc.Select("c", k.ID, ""); err != nil {
		t.Errorf("sign-in must still work for this run: %v", err)
	}
}

func TestSecret_NoRandomnessFailsClosed(t *testing.T) {
	r := newRig(t)
	r.secrets.m = map[string]string{}
	r.svc.random = func(int) ([]byte, error) { return nil, errors.New("no entropy") }
	r.svc.Load()
	if _, _, err := r.svc.Select("c", profile.EveryoneID, ""); err == nil {
		t.Error("a credential was issued with no secret")
	}
	if _, ok := r.svc.Resolve("everyone.1." + strings.Repeat("0", 64)); ok {
		t.Error("a credential verified with no secret")
	}
}

func TestVisible(t *testing.T) {
	items := []media.Item{
		{ID: "1", LibraryID: "cartoons", Kind: media.KindVideo},
		{ID: "2", LibraryID: "movies", Kind: media.KindVideo},
		{ID: "3", LibraryID: "cartoons", Kind: media.KindAudio},
	}
	got := Visible(kids(), items)
	if len(got) != 1 || got[0].ID != "1" {
		t.Errorf("Visible() = %+v", got)
	}
	if len(Visible(profile.Everyone(), items)) != 3 {
		t.Error("Everyone lost items")
	}
}

func TestTVVisible(t *testing.T) {
	r := newRig(t,
		media.Library{ID: "open", Root: "/a"},
		media.Library{ID: "adult", Root: "/b", HiddenFromTVs: true})
	for id, want := range map[media.LibraryID]bool{"open": true, "adult": false, "gone": false} {
		if got := r.svc.TVVisible(media.Item{LibraryID: id}); got != want {
			t.Errorf("TVVisible(%s) = %v, want %v", id, got, want)
		}
	}
}

func TestEntries_AreCopies(t *testing.T) {
	r := newRig(t)
	_, _ = r.svc.Save(kids())
	e := r.svc.Entries()
	e[1].Profile.Libraries[0] = "changed"
	if r.svc.Entries()[1].Profile.Libraries[0] != "cartoons" {
		t.Error("Entries() hands out the service's own slices")
	}
}

func TestConcurrentUse(t *testing.T) {
	r := newRig(t)
	k, _ := r.svc.Save(kids())
	cred, _, _ := r.svc.Select("c", k.ID, "")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				r.svc.Resolve(cred)
				r.svc.Summaries()
				r.svc.Active()
				_, _, _ = r.svc.Select("c", k.ID, "")
				_, _ = r.svc.Save(profile.Profile{ID: k.ID, Name: "Kids", Color: "green", AllLibraries: true})
			}
		}()
	}
	wg.Wait()
}

func TestPIN_UsesTheRealWorkFactorByDefault(t *testing.T) {
	s := NewService(&fakeStore{}, &fakeSecrets{m: map[string]string{}}, fakeLibs{}, &fakeEvents{}, slog.New(slog.DiscardHandler))
	if s.rounds != pinRounds || pinRounds < 100_000 {
		t.Errorf("rounds = %d", s.rounds)
	}
}
