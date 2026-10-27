package bridge

import (
	"context"
	"log/slog"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"eincast/internal/app"
)

// Event names the frontend subscribes to. Keep in sync with
// frontend/src/shared/bridge/events.ts.
const (
	EventLibraryScanned = "library:scanned"
	EventServerState    = "server:state"
	EventLogLine        = "log:line"
	EventLibraryStatus  = "library:status"
	EventSettings       = "settings:changed"
	EventRemote         = "remote:status"
	EventProfiles       = "profiles:changed"
)

// Emitter implements app.EventPublisher by forwarding events to the frontend.
// It is deliberately not bound to JS.
type Emitter struct {
	log  *slog.Logger
	emit func(ctx context.Context, name string, data ...any)

	mu  sync.RWMutex
	ctx context.Context
}

// NewEmitter returns an Emitter that emits through the Wails runtime once the
// app has started.
func NewEmitter(log *slog.Logger) *Emitter {
	return &Emitter{log: log, emit: wruntime.EventsEmit}
}

// emitLog forwards one log entry. It must never log, not even to report that
// the UI is not ready: a log line that triggers a log line is a feedback loop.
func (e *Emitter) emitLog(entry app.LogEntry) {
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()
	if ctx == nil {
		return
	}
	e.emit(ctx, EventLogLine, toLogLineDTO(entry))
}

func (e *Emitter) bind(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ctx = ctx
}

// Publish translates a domain-side event into a named, DTO-shaped frontend
// event. It never blocks on or fails because of the UI.
func (e *Emitter) Publish(ev app.Event) {
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()
	if ctx == nil {
		e.log.Warn("event dropped: UI not ready", slog.Any("event", ev))
		return
	}

	switch ev := ev.(type) {
	case app.LibraryScanned:
		e.emit(ctx, EventLibraryScanned, toLibraryStateDTOs(ev.Libraries))
	case app.LibraryStatusChanged:
		e.emit(ctx, EventLibraryStatus, toLibraryStateDTOs(ev.Libraries))
	case app.SettingsChanged:
		e.emit(ctx, EventSettings, toSettingsDTO(ev.View))
	case app.ProfilesChanged:
		e.emit(ctx, EventProfiles, toProfileDTOs(ev.Profiles))
	case app.RemoteStatusChanged:
		e.emit(ctx, EventRemote, toRemoteStatusDTO(ev.Status))
	case app.ServerStateChanged:
		e.emit(ctx, EventServerState, toServerStateDTO(ev.Status))
	default:
		e.log.Error("event has no frontend mapping", slog.Any("event", ev))
	}
}
