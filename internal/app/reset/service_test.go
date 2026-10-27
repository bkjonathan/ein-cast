package reset

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

type fakeData struct {
	calls int
	err   error
}

func (f *fakeData) Erase() error { f.calls++; return f.err }

type fakeSecrets struct {
	calls int
	err   error
}

func (f *fakeSecrets) EraseAll() error { f.calls++; return f.err }

type fakeLogin struct {
	set []bool
	err error
}

func (f *fakeLogin) Set(enabled bool) error { f.set = append(f.set, enabled); return f.err }

type fakeRelaunch struct {
	calls int
	err   error
}

func (f *fakeRelaunch) Relaunch() error { f.calls++; return f.err }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestErase(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		login       *fakeLogin // nil: no login entry on this platform
		data        error
		secrets     error
		loginErr    error
		wantErrPart []string
	}{
		{name: "everything goes", login: &fakeLogin{}},
		{name: "no login entry on this platform"},
		{name: "files fail", login: &fakeLogin{}, data: boom, wantErrPart: []string{"settings and caches", "boom"}},
		{name: "secrets fail", login: &fakeLogin{}, secrets: boom, wantErrPart: []string{"tokens and PINs"}},
		{name: "login entry fails", login: &fakeLogin{}, loginErr: boom, wantErrPart: []string{"open-at-login"}},
		{name: "all fail and all are reported", login: &fakeLogin{}, data: boom, secrets: boom, loginErr: boom,
			wantErrPart: []string{"open-at-login", "tokens and PINs", "settings and caches"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, secrets := &fakeData{err: tt.data}, &fakeSecrets{err: tt.secrets}
			var s *Service
			if tt.login != nil {
				tt.login.err = tt.loginErr
				s = NewService(data, secrets, tt.login, &fakeRelaunch{}, quiet())
			} else {
				s = NewService(data, secrets, nil, &fakeRelaunch{}, quiet())
			}

			err := s.Erase()

			// A failing step must not stop the others: that is the point of the test.
			if data.calls != 1 || secrets.calls != 1 {
				t.Errorf("steps run: files %d, secrets %d; want 1 each", data.calls, secrets.calls)
			}
			if tt.login != nil && (len(tt.login.set) != 1 || tt.login.set[0]) {
				t.Errorf("login entry calls = %v, want one Set(false)", tt.login.set)
			}
			if len(tt.wantErrPart) == 0 {
				if err != nil {
					t.Fatalf("Erase() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Erase() = nil, want an error")
			}
			for _, part := range tt.wantErrPart {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q does not mention %q", err, part)
				}
			}
		})
	}
}

func TestRelaunch(t *testing.T) {
	r := &fakeRelaunch{}
	s := NewService(&fakeData{}, &fakeSecrets{}, nil, r, quiet())
	if err := s.Relaunch(); err != nil || r.calls != 1 {
		t.Fatalf("Relaunch() = %v after %d calls", err, r.calls)
	}

	r.err = errors.New("no such program")
	err := s.Relaunch()
	if err == nil || !strings.Contains(err.Error(), "no such program") {
		t.Fatalf("Relaunch() = %v, want the cause wrapped", err)
	}
}
