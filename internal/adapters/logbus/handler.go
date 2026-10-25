package logbus

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"eincast/internal/app"
)

// maxValueLen caps one rendered attribute or message so a stack trace or a huge
// error cannot bloat the ring and every UI event.
const maxValueLen = 2000

// Handler is a slog.Handler decorator: it wraps the console handler, so
// existing call sites keep logging exactly as before, and also publishes every
// record to a Bus for the UI.
type Handler struct {
	base   slog.Handler
	bus    *Bus
	groups []string      // open WithGroup names, outermost first
	attrs  []app.LogAttr // WithAttrs values, already rendered with their group prefix
}

// NewHandler decorates base. The bus receives records from Debug up even when
// base filters them, so the UI's level chips have something to show.
func NewHandler(base slog.Handler, bus *Bus) *Handler {
	return &Handler{base: base, bus: bus}
}

// Enabled reports true for Debug and above, plus whatever base accepts.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelDebug || h.base.Enabled(ctx, level)
}

// Handle publishes the record, then forwards it to the console handler if that
// handler wants it.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	h.bus.Publish(h.entry(r))
	if !h.base.Enabled(ctx, r.Level) {
		return nil
	}
	return h.base.Handle(ctx, r)
}

// WithAttrs returns a handler that carries attrs on every record. Rendering them
// now, with the current group prefix, keeps Handle cheap.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	rendered := append([]app.LogAttr(nil), h.attrs...)
	prefix := h.prefix()
	for _, a := range attrs {
		rendered = appendAttr(rendered, prefix, a)
	}
	return &Handler{base: h.base.WithAttrs(attrs), bus: h.bus, groups: h.groups, attrs: rendered}
}

// WithGroup opens a group: later attributes get a "name." key prefix.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := append(append([]string(nil), h.groups...), name)
	return &Handler{base: h.base.WithGroup(name), bus: h.bus, groups: groups, attrs: h.attrs}
}

func (h *Handler) prefix() string {
	if len(h.groups) == 0 {
		return ""
	}
	return strings.Join(h.groups, ".") + "."
}

// entry converts a record. The "source" attribute is lifted into its own field
// (the UI shows it as a tag) and removed from the attribute list.
func (h *Handler) entry(r slog.Record) app.LogEntry {
	attrs := append([]app.LogAttr(nil), h.attrs...)
	prefix := h.prefix()
	r.Attrs(func(a slog.Attr) bool {
		attrs = appendAttr(attrs, prefix, a)
		return true
	})

	source := "app"
	kept := attrs[:0]
	for _, a := range attrs {
		if a.Key == "source" {
			source = a.Value
			continue
		}
		kept = append(kept, a)
	}

	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}
	return app.LogEntry{
		Time:    t,
		Level:   r.Level,
		Source:  source,
		Message: truncate(r.Message),
		Attrs:   kept,
	}
}

// appendAttr renders a (resolving LogValuers and flattening groups into
// "group.key") and appends it to dst.
func appendAttr(dst []app.LogAttr, prefix string, a slog.Attr) []app.LogAttr {
	if a.Equal(slog.Attr{}) {
		return dst // slog's rule: an attr with zero key and value is ignored
	}
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		inner := prefix
		if a.Key != "" {
			inner += a.Key + "."
		}
		for _, ga := range v.Group() {
			dst = appendAttr(dst, inner, ga)
		}
		return dst
	}
	return append(dst, app.LogAttr{Key: prefix + a.Key, Value: truncate(v.String())})
}

// truncate shortens s to maxValueLen bytes without splitting a UTF-8 character.
func truncate(s string) string {
	if len(s) <= maxValueLen {
		return s
	}
	cut := maxValueLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
