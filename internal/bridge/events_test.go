package bridge

import (
	"context"
	"log/slog"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
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
