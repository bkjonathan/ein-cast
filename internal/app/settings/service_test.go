package settings

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

type fakeStore struct {
	mu       sync.Mutex
	data     *app.Settings
	loadErr  error
	saveErr  error
	saves    []app.Settings
	loadCall int
}

func (f *fakeStore) Load() (app.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loadCall++
	if f.loadErr != nil {
		return app.Settings{}, f.loadErr
	}
	if f.data == nil {
		return app.Settings{}, fmt.Errorf("load: %w", app.ErrNoSettings)
	}
	return *f.data, nil
}

func (f *fakeStore) Save(s app.Settings) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves = append(f.saves, s)
	f.data = &s
	return nil
}

type fakeSecrets struct {
	m      map[string]string
	getErr error
	setErr error
	delErr error
	sets   int
}

func newSecrets() *fakeSecrets { return &fakeSecrets{m: map[string]string{}} }

func (f *fakeSecrets) Get(k string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.m[k]
	if !ok {
		return "", fmt.Errorf("get %q: %w", k, app.ErrSecretNotFound)
	}
	return v, nil
}
func (f *fakeSecrets) Set(k, v string) error {
	f.sets++
	if f.setErr != nil {
		return f.setErr
	}
	f.m[k] = v
	return nil
}
func (f *fakeSecrets) Delete(k string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.m, k)
	return nil
}

type fakeEvents struct {
	mu  sync.Mutex
	got []app.SettingsView
}

func (f *fakeEvents) Publish(ev app.Event) {
	if e, ok := ev.(app.SettingsChanged); ok {
		f.mu.Lock()
		f.got = append(f.got, e.View)
		f.mu.Unlock()
	}
}

// newTestService wires fakes and a token generator that yields t1, t2, ...
func newTestService(store *fakeStore, secrets *fakeSecrets) (*Service, *fakeEvents) {
	ev := &fakeEvents{}
	s := NewService(store, secrets, ev, slog.New(slog.DiscardHandler))
	n := 0
	s.newToken = func() (string, error) { n++; return fmt.Sprintf("t%d", n), nil }
	return s, ev
}

func TestLoad_FirstRunUsesDefaults(t *testing.T) {
	s, _ := newTestService(&fakeStore{}, newSecrets())
	s.Load()

	got := s.Get()
	if !reflect.DeepEqual(got.Settings, Default()) || got.Token != "" || s.Required() {
		t.Fatalf("Get() = %+v, want the defaults with no token", got)
	}
}

func TestLoad_RestoresSavedSettings(t *testing.T) {
	saved := app.Settings{Port: 9000, Name: "Den", Libraries: []media.Library{media.NewLibrary("/media")}}
	s, _ := newTestService(&fakeStore{data: &saved}, newSecrets())
	s.Load()

	if got := s.Get().Settings; !reflect.DeepEqual(got, saved) {
		t.Fatalf("Get() = %+v, want %+v", got, saved)
	}
	if libs := s.Libraries(); len(libs) != 1 || libs[0].Root != "/media" {
		t.Errorf("Libraries() = %+v", libs)
	}
}

func TestLoad_UnusableFileFallsBackToDefaults(t *testing.T) {
	s, _ := newTestService(&fakeStore{loadErr: errors.New("corrupt json")}, newSecrets())
	s.Load()
	if !reflect.DeepEqual(s.Get().Settings, Default()) {
		t.Fatalf("Get() = %+v, want defaults", s.Get().Settings)
	}
}

func TestLoad_ResetsOnlyTheInvalidFields(t *testing.T) {
	tests := []struct {
		name string
		in   app.Settings
		want app.Settings
	}{
		{"bad port keeps good name", app.Settings{Port: 70000, Name: "Den"}, app.Settings{Port: Default().Port, Name: "Den"}},
		{"privileged port", app.Settings{Port: 80, Name: "Den"}, app.Settings{Port: Default().Port, Name: "Den"}},
		{"bad name keeps good port", app.Settings{Port: 9000, Name: "a\nb"}, app.Settings{Port: 9000, Name: Default().Name}},
		{"empty name", app.Settings{Port: 9000}, app.Settings{Port: 9000, Name: Default().Name}},
		{"keeps the libraries and token flag", app.Settings{Port: 9000, Name: "x", Libraries: []media.Library{media.NewLibrary("/m")}, RequireToken: true}, app.Settings{Port: 9000, Name: "x", Libraries: []media.Library{media.NewLibrary("/m")}, RequireToken: true}},
		{"keeps the TV setting", app.Settings{Port: 9000, Name: "x", DLNA: true}, app.Settings{Port: 9000, Name: "x", DLNA: true}},
		{"a file asking for both keeps the token and drops the TV access", app.Settings{Port: 9000, Name: "x", RequireToken: true, DLNA: true}, app.Settings{Port: 9000, Name: "x", RequireToken: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newTestService(&fakeStore{data: &tt.in}, newSecrets())
			s.Load()
			if got := s.Get().Settings; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Get() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUpdate_SavesAndPublishes(t *testing.T) {
	store := &fakeStore{}
	s, ev := newTestService(store, newSecrets())
	s.Load()

	view, err := s.Update(Input{Port: 9100, Name: "  Living Room  "})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if view.Port != 9100 || view.Name != "Living Room" {
		t.Errorf("view = %+v (name must be trimmed)", view)
	}
	if len(store.saves) != 1 || store.saves[0].Port != 9100 {
		t.Errorf("saves = %+v", store.saves)
	}
	if len(ev.got) != 1 || ev.got[0].Port != 9100 {
		t.Errorf("events = %+v", ev.got)
	}
}

func TestUpdate_SmartTVDiscoveryIsSavedAndPublished(t *testing.T) {
	store := &fakeStore{}
	s, ev := newTestService(store, newSecrets())
	s.Load()
	if s.Get().DLNA {
		t.Fatal("DLNA is on by default: it opens the library to the whole network and must be asked for")
	}

	view, err := s.Update(Input{Port: 8765, Name: "EinCast", DLNA: true})
	if err != nil || !view.DLNA {
		t.Fatalf("Update() = %+v, %v", view, err)
	}
	if len(store.saves) != 1 || !store.saves[0].DLNA || len(ev.got) != 1 || !ev.got[0].DLNA {
		t.Errorf("saves=%+v events=%+v", store.saves, ev.got)
	}
}

func TestUpdate_TokenAndSmartTVDiscoveryCannotBothBeOn(t *testing.T) {
	store := &fakeStore{}
	s, ev := newTestService(store, newSecrets())
	s.Load()

	_, err := s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true, DLNA: true})
	if !errors.Is(err, ErrDLNAWithToken) {
		t.Fatalf("error = %v, want ErrDLNAWithToken", err)
	}
	if len(store.saves) != 0 || len(ev.got) != 0 || s.Get().RequireToken || s.Get().DLNA {
		t.Errorf("a refused update changed something: saves=%d events=%d view=%+v", len(store.saves), len(ev.got), s.Get())
	}

	// Either one alone is fine, and switching from one to the other is two steps.
	if _, err := s.Update(Input{Port: 8765, Name: "EinCast", DLNA: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true, DLNA: true}); !errors.Is(err, ErrDLNAWithToken) {
		t.Errorf("turning the token on while TVs are on: error = %v", err)
	}
	if _, err := s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true}); err != nil {
		t.Errorf("token on, TVs off: %v", err)
	}
}

func TestUpdate_ValidationFailuresChangeNothing(t *testing.T) {
	tests := []struct {
		name string
		in   Input
	}{
		{"port too low", Input{Port: 80, Name: "x"}},
		{"port too high", Input{Port: 65536, Name: "x"}},
		{"zero port", Input{Port: 0, Name: "x"}},
		{"blank name", Input{Port: 9000, Name: "   "}},
		{"control chars", Input{Port: 9000, Name: "a\x00b"}},
		{"name too long", Input{Port: 9000, Name: strings.Repeat("x", 65)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{}
			s, ev := newTestService(store, newSecrets())
			s.Load()
			before := s.Get()

			if _, err := s.Update(tt.in); err == nil {
				t.Fatal("Update() error = nil")
			}
			if !reflect.DeepEqual(s.Get(), before) || len(store.saves) != 0 || len(ev.got) != 0 {
				t.Errorf("a rejected update had effects: state=%+v saves=%d events=%d", s.Get(), len(store.saves), len(ev.got))
			}
		})
	}
}

func TestUpdate_SaveFailureChangesNothing(t *testing.T) {
	boom := errors.New("disk full")
	store := &fakeStore{saveErr: boom}
	s, ev := newTestService(store, newSecrets())
	s.Load()

	if _, err := s.Update(Input{Port: 9000, Name: "x", RequireToken: true}); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if s.Get().Port != Default().Port || s.Required() || len(ev.got) != 0 {
		t.Errorf("state changed despite the failed save: %+v required=%v", s.Get(), s.Required())
	}
}

func TestToken_EnableDisableLifecycle(t *testing.T) {
	secrets := newSecrets()
	s, ev := newTestService(&fakeStore{}, secrets)
	s.Load()
	if !s.Verify("") || !s.Verify("anything") {
		t.Fatal("with the token off every request must be allowed")
	}

	view, err := s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true})
	if err != nil {
		t.Fatal(err)
	}
	if view.Token != "t1" || !view.TokenPersisted || secrets.m[tokenKey] != "t1" {
		t.Fatalf("view = %+v secrets = %v, want a new persisted token t1", view, secrets.m)
	}
	if !s.Required() || !s.Verify("t1") || s.Verify("t2") || s.Verify("") {
		t.Error("with the token on only the exact token may pass")
	}

	if _, err := s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: false}); err != nil {
		t.Fatal(err)
	}
	if _, kept := secrets.m[tokenKey]; kept {
		t.Error("turning the token off must delete the saved secret")
	}
	if got := s.Get().Token; got != "" {
		t.Errorf("token still exposed after turning off: %q", got)
	}
	if !s.Verify("t1") { // off means open again; the old token is simply irrelevant
		t.Error("with the token off every request must be allowed again")
	}
	if len(ev.got) != 2 {
		t.Errorf("published %d events, want 2", len(ev.got))
	}

	// Turning on again never resurrects the old token.
	view, _ = s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true})
	if view.Token != "t2" {
		t.Errorf("re-enabled token = %q, want a fresh t2", view.Token)
	}
}

func TestToken_UpdateWithoutTogglingKeepsTheToken(t *testing.T) {
	s, _ := newTestService(&fakeStore{}, newSecrets())
	s.Load()
	_, _ = s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true})

	view, _ := s.Update(Input{Port: 9001, Name: "Renamed", RequireToken: true})
	if view.Token != "t1" {
		t.Errorf("token = %q, want it unchanged by an unrelated edit", view.Token)
	}
}

func TestToken_LoadedFromTheCredentialStore(t *testing.T) {
	secrets := newSecrets()
	secrets.m[tokenKey] = "saved-token"
	saved := app.Settings{Port: 8765, Name: "EinCast", RequireToken: true}
	s, _ := newTestService(&fakeStore{data: &saved}, secrets)
	s.Load()

	if got := s.Get(); got.Token != "saved-token" || !got.TokenPersisted {
		t.Fatalf("Get() = %+v, want the saved token", got)
	}
	if !s.Verify("saved-token") {
		t.Error("the saved token must verify after a restart")
	}
	if secrets.sets != 0 {
		t.Error("an existing token must not be rewritten")
	}
}

func TestToken_MissingSecretIsCreatedOnLoad(t *testing.T) {
	secrets := newSecrets()
	saved := app.Settings{Port: 8765, Name: "EinCast", RequireToken: true}
	s, _ := newTestService(&fakeStore{data: &saved}, secrets)
	s.Load()

	if secrets.m[tokenKey] != "t1" || s.Get().Token != "t1" {
		t.Fatalf("secrets = %v, view = %+v", secrets.m, s.Get())
	}
}

// A locked or broken keychain must neither disable protection nor overwrite a
// token that is still there.
func TestToken_UnavailableCredentialStoreFailsSafe(t *testing.T) {
	secrets := newSecrets()
	secrets.m[tokenKey] = "precious"
	secrets.getErr = errors.New("keychain is locked")
	saved := app.Settings{Port: 8765, Name: "EinCast", RequireToken: true}
	s, _ := newTestService(&fakeStore{data: &saved}, secrets)
	s.Load()

	if !s.Required() {
		t.Fatal("protection must stay on when the credential store is unavailable")
	}
	got := s.Get()
	if got.Token == "" || got.TokenPersisted {
		t.Errorf("Get() = %+v, want a temporary, non-persisted token", got)
	}
	if secrets.sets != 0 || secrets.m[tokenKey] != "precious" {
		t.Errorf("the saved token was overwritten: sets=%d value=%q", secrets.sets, secrets.m[tokenKey])
	}
}

func TestToken_UnsavableTokenIsReportedAsNotPersisted(t *testing.T) {
	secrets := newSecrets()
	secrets.setErr = errors.New("denied")
	s, _ := newTestService(&fakeStore{}, secrets)
	s.Load()

	view, err := s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true})
	if err != nil {
		t.Fatalf("Update() error = %v; a keychain problem must not block the feature", err)
	}
	if view.Token == "" || view.TokenPersisted {
		t.Errorf("view = %+v, want a working token flagged as not persisted", view)
	}
	if !s.Verify(view.Token) {
		t.Error("the temporary token must still work")
	}
}

func TestVerify_FailsClosedWithoutAToken(t *testing.T) {
	s, _ := newTestService(&fakeStore{}, newSecrets())
	s.newToken = func() (string, error) { return "", errors.New("no entropy") }
	s.Load()
	_, _ = s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true})

	if !s.Required() {
		t.Fatal("must stay required")
	}
	for _, candidate := range []string{"", "x"} {
		if s.Verify(candidate) {
			t.Errorf("Verify(%q) = true with no token available; it must fail closed", candidate)
		}
	}
}

func TestRotateToken(t *testing.T) {
	secrets := newSecrets()
	s, ev := newTestService(&fakeStore{}, secrets)
	s.Load()

	if _, err := s.RotateToken(); !errors.Is(err, ErrTokenOff) {
		t.Fatalf("RotateToken() with the token off: error = %v, want ErrTokenOff", err)
	}

	_, _ = s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true})
	view, err := s.RotateToken()
	if err != nil {
		t.Fatal(err)
	}
	if view.Token != "t2" || secrets.m[tokenKey] != "t2" {
		t.Errorf("view = %+v secrets = %v", view, secrets.m)
	}
	if s.Verify("t1") || !s.Verify("t2") {
		t.Error("after rotation the old token must stop working and the new one work")
	}
	if len(ev.got) != 2 || ev.got[1].Token != "t2" {
		t.Errorf("events = %+v", ev.got)
	}
}

func TestSaveLibraries(t *testing.T) {
	store := &fakeStore{}
	s, _ := newTestService(store, newSecrets())
	s.Load()
	films, music := media.NewLibrary("/media/films"), media.NewLibrary("/media/music")

	if err := s.SaveLibraries([]media.Library{films, music}); err != nil {
		t.Fatal(err)
	}
	if got := s.Libraries(); !reflect.DeepEqual(got, []media.Library{films, music}) || len(store.saves) != 1 ||
		!reflect.DeepEqual(store.saves[0].Libraries, []media.Library{films, music}) {
		t.Errorf("Libraries()=%+v saves=%+v", got, store.saves)
	}

	store.saveErr = errors.New("read-only disk")
	if err := s.SaveLibraries([]media.Library{films}); err == nil || len(s.Libraries()) != 2 {
		t.Errorf("a failed save must report the error and keep the old list (err=%v libraries=%+v)", err, s.Libraries())
	}
}

func TestSaveLibraries_ReturnsACopy(t *testing.T) {
	s, _ := newTestService(&fakeStore{}, newSecrets())
	s.Load()
	in := []media.Library{media.NewLibrary("/a")}
	_ = s.SaveLibraries(in)

	in[0].Name = "changed by the caller"
	s.Libraries()[0].Name = "changed through the result"
	if got := s.Libraries()[0].Name; got != "a" {
		t.Errorf("Name = %q: the stored list was changed through a shared slice", got)
	}
}

func TestSaveLibraries_PreservesOtherSettings(t *testing.T) {
	store := &fakeStore{}
	s, _ := newTestService(store, newSecrets())
	s.Load()
	_, _ = s.Update(Input{Port: 9100, Name: "Den", DLNA: true})
	_ = s.SaveLibraries([]media.Library{media.NewLibrary("/media")})

	last := store.saves[len(store.saves)-1]
	if last.Port != 9100 || last.Name != "Den" || !last.DLNA || len(last.Libraries) != 1 {
		t.Errorf("last save = %+v; remembering the folders must not reset other settings", last)
	}
}

func TestGenerateToken(t *testing.T) {
	a, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := generateToken()
	if len(a) != 32 || a == b {
		t.Errorf("tokens %q, %q: want two different 32-character values (192 bits)", a, b)
	}
	for _, c := range a {
		ok := c == '-' || c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		if !ok {
			t.Fatalf("token %q contains %q, which is not URL-safe", a, c)
		}
	}
}

// Run with -race.
func TestConcurrentUse(t *testing.T) {
	s, _ := newTestService(&fakeStore{}, newSecrets())
	s.Load()
	_, _ = s.Update(Input{Port: 8765, Name: "EinCast", RequireToken: true})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = s.Verify("x")
				_ = s.Get()
			}
		}()
		go func() {
			defer wg.Done()
			for range 20 {
				_, _ = s.RotateToken()
			}
		}()
		go func() {
			defer wg.Done()
			for range 20 {
				_ = s.SaveLibraries([]media.Library{media.NewLibrary("/a")})
				_ = s.SaveLibraries([]media.Library{media.NewLibrary("/b")})
			}
		}()
	}
	wg.Wait()
}
