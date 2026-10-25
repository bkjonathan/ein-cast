package logbus

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"eincast/internal/app"
)

// newLogger builds Handler(console at Info) -> Bus, returning the console text.
func newLogger() (*slog.Logger, *Bus, *bytes.Buffer) {
	console := &bytes.Buffer{}
	base := slog.NewTextHandler(console, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				return slog.Attr{} // deterministic console output
			}
			return a
		},
	})
	bus := NewBus(100)
	return slog.New(NewHandler(base, bus)), bus, console
}

func last(t *testing.T, b *Bus) app.LogEntry {
	t.Helper()
	h := b.History()
	if len(h) == 0 {
		t.Fatal("bus is empty")
	}
	return h[len(h)-1]
}

func attrMap(e app.LogEntry) map[string]string {
	m := map[string]string{}
	for _, a := range e.Attrs {
		m[a.Key] = a.Value
	}
	return m
}

func TestHandler_ConsoleOutputIsUnchanged(t *testing.T) {
	log, _, console := newLogger()
	log.Info("hello", slog.String("path", "/a"))

	if got, want := strings.TrimSpace(console.String()), `level=INFO msg=hello path=/a`; got != want {
		t.Fatalf("console = %q, want %q (the decorator must not alter existing output)", got, want)
	}
}

func TestHandler_PublishesRecordFields(t *testing.T) {
	log, bus, _ := newLogger()
	before := time.Now()
	log.Warn("disk low", slog.Int("pct", 7), slog.Duration("took", 1500*time.Millisecond), slog.Bool("ok", false))

	e := last(t, bus)
	if e.Message != "disk low" || e.Level != slog.LevelWarn || e.Source != "app" {
		t.Errorf("entry = %+v", e)
	}
	if e.Time.Before(before) || e.Time.After(time.Now()) {
		t.Errorf("Time = %v, want a time around now", e.Time)
	}
	want := map[string]string{"pct": "7", "took": "1.5s", "ok": "false"}
	got := attrMap(e)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attr %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	// Order is preserved: the UI shows attributes as the caller wrote them.
	if len(e.Attrs) != 3 || e.Attrs[0].Key != "pct" || e.Attrs[2].Key != "ok" {
		t.Errorf("attrs order = %+v", e.Attrs)
	}
}

func TestHandler_DebugReachesTheUIEvenThoughConsoleIsInfo(t *testing.T) {
	log, bus, console := newLogger()
	log.Debug("noisy detail")

	if e := last(t, bus); e.Message != "noisy detail" || e.Level != slog.LevelDebug {
		t.Errorf("entry = %+v", e)
	}
	if console.Len() != 0 {
		t.Errorf("console printed %q; its own level (Info) must still apply", console.String())
	}
}

func TestHandler_SourceIsLiftedOutOfAttrs(t *testing.T) {
	log, bus, _ := newLogger()
	log.Info("request", slog.String("source", "http"), slog.Int("status", 200))

	e := last(t, bus)
	if e.Source != "http" {
		t.Errorf("Source = %q, want http", e.Source)
	}
	if _, dup := attrMap(e)["source"]; dup {
		t.Error("source should not repeat in Attrs")
	}
}

func TestHandler_WithAttrsAndGroups(t *testing.T) {
	log, bus, console := newLogger()

	tagged := log.With(slog.String("source", "library"), slog.String("root", "/m"))
	tagged.Info("scanned", slog.Int("items", 3))
	e := last(t, bus)
	if e.Source != "library" || attrMap(e)["root"] != "/m" || attrMap(e)["items"] != "3" {
		t.Errorf("With(): entry = %+v", e)
	}

	log.WithGroup("req").With(slog.String("id", "abc")).Info("done", slog.Int("status", 200))
	e = last(t, bus)
	if got := attrMap(e); got["req.id"] != "abc" || got["req.status"] != "200" {
		t.Errorf("WithGroup(): attrs = %v, want req.id and req.status", got)
	}

	log.Info("inline", slog.Group("db", slog.String("name", "x"), slog.Group("conn", slog.Int("n", 2))))
	if got := attrMap(last(t, bus)); got["db.name"] != "x" || got["db.conn.n"] != "2" {
		t.Errorf("slog.Group: attrs = %v", got)
	}

	// The console still sees the same grouping.
	if !strings.Contains(console.String(), "req.id=abc") {
		t.Errorf("console lost the group: %q", console.String())
	}

	// A group name does not apply to a record logged through the parent logger.
	log.Info("plain", slog.String("id", "z"))
	if _, leaked := attrMap(last(t, bus))["req.id"]; leaked {
		t.Error("group leaked into an unrelated logger")
	}
}

func TestHandler_ValueRendering(t *testing.T) {
	log, bus, _ := newLogger()
	log.LogAttrs(context.Background(), slog.LevelError, "boom",
		slog.Any("err", errors.New("disk on fire")), slog.Any("nil", nil), slog.Attr{})

	e := last(t, bus)
	if got := attrMap(e)["err"]; got != "disk on fire" {
		t.Errorf("err = %q", got)
	}
	if got := attrMap(e)["nil"]; got != "<nil>" {
		t.Errorf("nil = %q", got)
	}
	if len(e.Attrs) != 2 {
		t.Errorf("attrs = %+v; the zero Attr must be ignored", e.Attrs)
	}
}

type secretValuer struct{}

func (secretValuer) LogValue() slog.Value { return slog.StringValue("resolved") }

func TestHandler_ResolvesLogValuers(t *testing.T) {
	log, bus, _ := newLogger()
	log.Info("x", slog.Any("v", secretValuer{}))
	if got := attrMap(last(t, bus))["v"]; got != "resolved" {
		t.Errorf("v = %q, want the LogValuer's resolved value", got)
	}
}

func TestHandler_TruncatesHugeValues(t *testing.T) {
	log, bus, _ := newLogger()
	huge := strings.Repeat("é", maxValueLen) // 2 bytes per rune: cutting must not split one
	log.Info(huge, slog.String("stack", huge))

	e := last(t, bus)
	for name, s := range map[string]string{"message": e.Message, "attr": attrMap(e)["stack"]} {
		if len(s) > maxValueLen+len("…") {
			t.Errorf("%s has %d bytes, want at most %d", name, len(s), maxValueLen+len("…"))
		}
		if !strings.HasSuffix(s, "…") {
			t.Errorf("%s is not marked as truncated", name)
		}
		if !utf8.ValidString(s) {
			t.Errorf("%s is not valid UTF-8: truncation split a rune", name)
		}
	}
}

func TestHandler_Enabled(t *testing.T) {
	bus := NewBus(1)
	base := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError})
	h := NewHandler(base, bus)
	ctx := context.Background()

	for _, l := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if !h.Enabled(ctx, l) {
			t.Errorf("Enabled(%v) = false, want true", l)
		}
	}
	if h.Enabled(ctx, slog.LevelDebug-4) && !base.Enabled(ctx, slog.LevelDebug-4) {
		t.Error("levels below Debug should only pass if the base accepts them")
	}
}

func TestHandler_DecoratorSurvivesDerivedLoggersAndConcurrency(t *testing.T) {
	log, bus, _ := newLogger()
	done := make(chan struct{})
	for range 4 {
		go func() {
			l := log.With("source", "x")
			for range 100 {
				l.Info("m", "k", "v")
			}
			done <- struct{}{}
		}()
	}
	for range 4 {
		<-done
	}
	if got := len(bus.History()); got != 100 {
		t.Errorf("History() = %d entries, want 100 (ring capacity)", got)
	}
}
