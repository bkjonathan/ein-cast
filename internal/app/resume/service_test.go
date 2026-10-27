package resume

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
)

type fakeStore struct {
	mu      sync.Mutex
	saved   map[profile.ID]map[media.ID]app.ResumePosition
	saves   int
	loadErr error
	saveErr error
}

func (f *fakeStore) Load() (map[profile.ID]map[media.ID]app.ResumePosition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.saved, nil
}

func (f *fakeStore) Save(d map[profile.ID]map[media.ID]app.ResumePosition) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves++
	f.saved = d
	return nil
}

func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saves
}

func newSvc(store *fakeStore) (*Service, *time.Time) {
	now := time.UnixMilli(1_700_000_000_000)
	s := NewService(store, slog.New(slog.DiscardHandler))
	s.now = func() time.Time { return now }
	s.delay = 20 * time.Millisecond
	return s, &now
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPut_ThenGet(t *testing.T) {
	s, _ := newSvc(&fakeStore{})
	if err := s.Put("p", "a", 754, 3600, 99); err != nil {
		t.Fatal(err)
	}
	got := s.Get("p")["a"]
	if got.T != 754 || got.D != 3600 || got.M != 99 || got.S != 1_700_000_000_000 {
		t.Errorf("position = %+v, want the server's own clock in S", got)
	}
	if len(s.Get("other")) != 0 {
		t.Error("another profile sees it")
	}
}

func TestPut_RefusesPositionsThatCannotBeReal(t *testing.T) {
	s, _ := newSvc(&fakeStore{})
	for name, c := range map[string][2]int{
		"negative": {-1, 100}, "past the end": {101, 100}, "no length": {0, 0},
		"an absurd length": {5, maxDuration + 1},
	} {
		if err := s.Put("p", "a", c[0], c[1], 1); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(s.Get("p")) != 0 {
		t.Error("a refused position was stored")
	}
}

func TestGet_ReturnsACopy(t *testing.T) {
	s, _ := newSvc(&fakeStore{})
	_ = s.Put("p", "a", 10, 100, 1)
	m := s.Get("p")
	delete(m, "a")
	if len(s.Get("p")) != 1 {
		t.Error("Get() hands out the service's own map")
	}
}

func TestWritesAreGatheredIntoOne(t *testing.T) {
	store := &fakeStore{}
	s, _ := newSvc(store)
	for i := 0; i < 50; i++ {
		_ = s.Put("p", "a", i, 100, 1)
	}
	eventually(t, func() bool { return store.count() >= 1 })
	time.Sleep(60 * time.Millisecond)
	if n := store.count(); n != 1 {
		t.Errorf("the file was written %d times for 50 saves in a burst, want 1", n)
	}
	if store.saved["p"]["a"].T != 49 {
		t.Errorf("saved %+v, want the last position", store.saved["p"]["a"])
	}
}

func TestClose_WritesWhatIsStillWaiting(t *testing.T) {
	store := &fakeStore{}
	s, _ := newSvc(store)
	s.delay = time.Hour
	_ = s.Put("p", "a", 10, 100, 1)
	if store.count() != 0 {
		t.Fatal("written too early")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if store.count() != 1 || store.saved["p"]["a"].T != 10 {
		t.Errorf("Close() did not save the last position: %+v", store.saved)
	}
	if err := s.Close(); err != nil || store.count() != 1 {
		t.Errorf("a second Close() wrote again: %v", err)
	}
}

func TestDeleteAndForget(t *testing.T) {
	store := &fakeStore{}
	s, _ := newSvc(store)
	_ = s.Put("p", "a", 10, 100, 1)
	_ = s.Put("p", "b", 10, 100, 1)
	_ = s.Put("q", "a", 10, 100, 1)

	s.Delete("p", "a")
	s.Delete("p", "never-there") // nothing to do, nothing to save
	if _, ok := s.Get("p")["a"]; ok || len(s.Get("p")) != 1 {
		t.Errorf("Delete: %+v", s.Get("p"))
	}
	s.Forget("p")
	if len(s.Get("p")) != 0 || len(s.Get("q")) != 1 {
		t.Errorf("Forget removed the wrong things: p=%v q=%v", s.Get("p"), s.Get("q"))
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.saved["p"]; ok {
		t.Error("a deleted profile's positions were saved")
	}
}

func TestPut_KeepsAProfileBounded(t *testing.T) {
	s, now := newSvc(&fakeStore{})
	s.delay = time.Hour
	for i := 0; i <= maxPerProfile; i++ {
		*now = now.Add(time.Millisecond)
		_ = s.Put("p", media.ID(fmt.Sprint(i)), 1, 10, 1)
	}
	got := s.Get("p")
	if len(got) != maxPerProfile {
		t.Fatalf("kept %d positions, want %d", len(got), maxPerProfile)
	}
	if _, ok := got["0"]; ok {
		t.Error("the oldest position was kept and a newer one dropped")
	}
}

func TestLoad(t *testing.T) {
	store := &fakeStore{saved: map[profile.ID]map[media.ID]app.ResumePosition{"p": {"a": {T: 5, D: 50}}}}
	s, _ := newSvc(store)
	s.Load()
	if s.Get("p")["a"].T != 5 {
		t.Error("saved positions were not loaded")
	}

	bad, _ := newSvc(&fakeStore{loadErr: errors.New("corrupt")})
	bad.Load() // must not panic or refuse
	if err := bad.Put("p", "a", 1, 10, 1); err != nil {
		t.Errorf("an unreadable file made the service unusable: %v", err)
	}
}

func TestNewest(t *testing.T) {
	m := map[media.ID]app.ResumePosition{"old": {S: 1}, "new": {S: 9}, "mid": {S: 5}, "tie-b": {S: 3}, "tie-a": {S: 3}}
	got := Newest(m)
	want := []media.ID{"new", "mid", "tie-a", "tie-b", "old"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Newest() = %v, want %v", got, want)
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	s, _ := newSvc(&fakeStore{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = s.Put("p", media.ID(fmt.Sprint(j%10)), j%50, 100, 1)
				s.Get("p")
				if j%25 == 0 {
					s.Delete("p", "1")
				}
			}
		}()
	}
	wg.Wait()
	_ = s.Close()
}
