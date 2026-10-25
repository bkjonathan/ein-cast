package bridge

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

type fakeLibrary struct {
	scanErr    error
	scanned    []string
	summary    app.LibrarySummary
	hasSummary bool
}

func (f *fakeLibrary) ScanFolder(_ context.Context, path string) (app.LibrarySummary, error) {
	f.scanned = append(f.scanned, path)
	return f.summary, f.scanErr
}

func (f *fakeLibrary) Summary() (app.LibrarySummary, bool) { return f.summary, f.hasSummary }

func newTestApp(lib *fakeLibrary) *App {
	a := New(Info{Name: "EinCast", Version: "1.2.3"}, lib, NewEmitter(slog.New(slog.DiscardHandler)))
	a.ctx = context.Background()
	return a
}

func TestAppInfo(t *testing.T) {
	got := newTestApp(&fakeLibrary{}).AppInfo()

	want := AppInfoDTO{Name: "EinCast", Version: "1.2.3", Platform: runtime.GOOS}
	if got != want {
		t.Fatalf("AppInfo() = %+v, want %+v", got, want)
	}
}

func TestChooseFolder(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		pickPath    string
		pickErr     error
		scanErr     error
		wantErr     error
		wantScanned []string
	}{
		{name: "picked and scanned", pickPath: "/media", wantScanned: []string{"/media"}},
		{name: "dialog cancelled is not an error", pickPath: ""},
		{name: "dialog fails", pickErr: boom, wantErr: boom},
		{name: "scan fails", pickPath: "/media", scanErr: boom, wantErr: boom, wantScanned: []string{"/media"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := &fakeLibrary{scanErr: tt.scanErr}
			a := newTestApp(lib)
			a.pickFolder = func(context.Context) (string, error) { return tt.pickPath, tt.pickErr }

			err := a.ChooseFolder()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ChooseFolder() error = %v, want %v", err, tt.wantErr)
			}
			if len(lib.scanned) != len(tt.wantScanned) || (len(lib.scanned) > 0 && lib.scanned[0] != tt.wantScanned[0]) {
				t.Fatalf("scanned = %v, want %v", lib.scanned, tt.wantScanned)
			}
		})
	}
}

func TestGetLibrarySummary(t *testing.T) {
	t.Run("nil before first scan", func(t *testing.T) {
		if got := newTestApp(&fakeLibrary{}).GetLibrarySummary(); got != nil {
			t.Fatalf("GetLibrarySummary() = %+v, want nil", got)
		}
	})

	t.Run("maps to DTO", func(t *testing.T) {
		lib := &fakeLibrary{hasSummary: true, summary: app.LibrarySummary{
			Root: "/m",
			Stats: media.Stats{Total: 3, TotalSize: 99, Counts: map[media.Kind]int{
				media.KindVideo: 2, media.KindImage: 1,
			}},
		}}
		got := newTestApp(lib).GetLibrarySummary()
		want := LibrarySummaryDTO{Root: "/m", Total: 3, TotalSize: 99, Counts: KindCountsDTO{Video: 2, Image: 1}}
		if got == nil || *got != want {
			t.Fatalf("GetLibrarySummary() = %+v, want %+v", got, want)
		}
	})
}
