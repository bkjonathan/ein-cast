package bridge

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/server"
)

type emitted struct {
	name string
	data []any
}

func newTestEmitter(rec *[]emitted) *Emitter {
	e := NewEmitter(slog.New(slog.DiscardHandler))
	e.emit = func(_ context.Context, name string, data ...any) {
		*rec = append(*rec, emitted{name, data})
	}
	return e
}

func TestEmitter_LibraryScanned(t *testing.T) {
	var got []emitted
	e := newTestEmitter(&got)
	e.bind(context.Background())

	e.Publish(app.LibraryScanned{Summary: app.LibrarySummary{
		Root:  "/m",
		Stats: media.Stats{Total: 1, TotalSize: 7, Counts: map[media.Kind]int{media.KindAudio: 1}},
	}})

	if len(got) != 1 || got[0].name != "library:scanned" || len(got[0].data) != 1 {
		t.Fatalf("emitted = %+v", got)
	}
	want := LibrarySummaryDTO{Root: "/m", Total: 1, TotalSize: 7, Counts: KindCountsDTO{Audio: 1}}
	if dto, ok := got[0].data[0].(LibrarySummaryDTO); !ok || dto != want {
		t.Fatalf("payload = %#v, want %#v", got[0].data[0], want)
	}
}

func TestEmitter_DropsEventsBeforeStartup(t *testing.T) {
	var got []emitted
	newTestEmitter(&got).Publish(app.LibraryScanned{})
	if len(got) != 0 {
		t.Fatalf("emitted %d events before the UI was ready, want 0", len(got))
	}
}

func TestEmitter_ServerStateChanged(t *testing.T) {
	var got []emitted
	e := newTestEmitter(&got)
	e.bind(context.Background())

	e.Publish(app.ServerStateChanged{Status: app.ServerStatus{
		State: server.Running, Name: "Den", Port: 8765,
		LocalURL: "http://127.0.0.1:8765", LANURLs: []string{"http://192.168.1.20:8765"},
	}})

	if len(got) != 1 || got[0].name != "server:state" {
		t.Fatalf("emitted = %+v", got)
	}
	want := ServerStateDTO{
		State: "running", Name: "Den", Port: 8765,
		LocalURL: "http://127.0.0.1:8765", LANURLs: []string{"http://192.168.1.20:8765"},
	}
	if dto, ok := got[0].data[0].(ServerStateDTO); !ok || !reflect.DeepEqual(dto, want) {
		t.Fatalf("payload = %#v, want %#v", got[0].data[0], want)
	}
}

// A log line that produced a log line would loop forever, so emitLog must stay
// silent when the UI is not ready. The logger here would fail the test if used.
func TestEmitLog_SilentBeforeStartup(t *testing.T) {
	var logged bytes.Buffer
	e := NewEmitter(slog.New(slog.NewTextHandler(&logged, nil)))
	var got []emitted
	e.emit = func(_ context.Context, name string, data ...any) { got = append(got, emitted{name, data}) }

	e.emitLog(app.LogEntry{Message: "early"})

	if len(got) != 0 {
		t.Errorf("emitted %d events before startup, want 0", len(got))
	}
	if logged.Len() != 0 {
		t.Errorf("emitLog logged %q; it must never log (feedback loop)", logged.String())
	}
}

func TestEmitter_LibraryStatusAndSettings(t *testing.T) {
	var got []emitted
	e := newTestEmitter(&got)
	e.bind(context.Background())

	e.Publish(app.LibraryStatusChanged{Status: app.LibraryStatus{Root: "/m", Error: "gone"}})
	e.Publish(app.SettingsChanged{View: app.SettingsView{
		Settings: app.Settings{Port: 9000, Name: "Den", RequireToken: true, LibraryRoot: "/private"},
		Token:    "tok", TokenPersisted: true,
	}})

	if len(got) != 2 || got[0].name != "library:status" || got[1].name != "settings:changed" {
		t.Fatalf("emitted = %+v", got)
	}
	if dto, _ := got[0].data[0].(LibraryStatusDTO); dto != (LibraryStatusDTO{Root: "/m", Error: "gone"}) {
		t.Errorf("status payload = %+v", dto)
	}
	want := SettingsDTO{Port: 9000, Name: "Den", RequireToken: true, AccessToken: "tok", TokenPersisted: true}
	if dto, _ := got[1].data[0].(SettingsDTO); dto != want {
		t.Errorf("settings payload = %+v, want %+v (and no library path)", dto, want)
	}
}
