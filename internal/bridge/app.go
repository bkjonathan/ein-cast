// Package bridge is the only package (besides main) that may import the Wails
// runtime. It exposes a small facade to the frontend and translates between
// JS-safe DTOs and the application layer.
package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"eincast/internal/app"
	"eincast/internal/app/streaming"
	"eincast/internal/domain/server"
)

// Info is the static identity of the running build, supplied by main.
type Info struct {
	Name    string
	Version string
}

// Library is the part of the library use cases the bridge needs. It is declared
// here, at the consumer, so the bridge can be tested with a fake.
type Library interface {
	ScanFolder(ctx context.Context, path string) (app.LibrarySummary, error)
	Summary() (app.LibrarySummary, bool)
}

// Streaming is the part of the server lifecycle the bridge drives.
type Streaming interface {
	Start(ctx context.Context, cfg streaming.Config) error
	Stop(ctx context.Context) error
	Status() app.ServerStatus
}

// App is the Wails-bound facade. Every exported method is callable from JS, so
// keep this surface small and stable (and keep lifecycle hooks unexported).
type App struct {
	info      Info
	library   Library
	streaming Streaming
	logs      app.LogStream
	emitter   *Emitter
	log       *slog.Logger

	// ctx lives for the whole app run and is cancelled on shutdown. Wails
	// binding calls carry no context of their own, so long operations use this.
	ctx context.Context
	// pickFolder opens the native directory dialog. A field so tests can
	// replace the one thing that needs a real window.
	pickFolder func(ctx context.Context) (string, error)
}

// Deps is everything the facade needs, passed in by main. Nothing is global.
type Deps struct {
	Info      Info
	Library   Library
	Streaming Streaming
	Logs      app.LogStream
	Emitter   *Emitter
	Log       *slog.Logger
}

// New builds the facade from its dependencies.
func New(d Deps) *App {
	return &App{
		info:       d.Info,
		library:    d.Library,
		streaming:  d.Streaming,
		logs:       d.Logs,
		emitter:    d.Emitter,
		log:        d.Log,
		pickFolder: openFolderDialog,
	}
}

// OnStartup returns the hook to give to wails.Options.OnStartup. It is a
// function rather than an exported method because exported methods are bound
// to JS.
func OnStartup(a *App) func(ctx context.Context) {
	return a.startup
}

// OnShutdown returns the hook to give to wails.Options.OnShutdown. It stops the
// server gracefully so in-flight streams finish and the port is released.
func OnShutdown(a *App) func(ctx context.Context) {
	return a.shutdown
}

func (a *App) shutdown(ctx context.Context) {
	// The app context may already be cancelled; shutdown must still run.
	if err := a.streaming.Stop(context.WithoutCancel(ctx)); err != nil && !errors.Is(err, server.ErrInvalidTransition) {
		// Nothing more to do while quitting, but leave a trace.
		a.log.Warn("stop server on shutdown", slog.String("error", err.Error()))
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emitter.bind(ctx)
	// Owned by the app context: the goroutine ends, and unsubscribes, when the
	// app shuts down.
	go a.forwardLogs(ctx)
}

// logForwardBuffer is how many entries may queue between the logger and the
// UI. If the UI falls further behind, the bus drops entries rather than block.
const logForwardBuffer = 512

// forwardLogs pushes every new log entry to the frontend as a "log:line" event
// until ctx is cancelled.
func (a *App) forwardLogs(ctx context.Context) {
	live, cancel := a.logs.Subscribe(logForwardBuffer)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-live:
			if !ok {
				return
			}
			a.emitter.emitLog(entry)
		}
	}
}

// AppInfo returns the app name, version and host platform.
func (a *App) AppInfo() AppInfoDTO {
	return AppInfoDTO{
		Name:     a.info.Name,
		Version:  a.info.Version,
		Platform: runtime.GOOS,
	}
}

// ChooseFolder opens the native folder picker and scans the chosen folder. The
// result arrives as a "library:scanned" event; this method only reports
// failure. Cancelling the dialog is not an error.
func (a *App) ChooseFolder() error {
	path, err := a.pickFolder(a.ctx)
	if err != nil {
		return fmt.Errorf("choose folder: %w", err)
	}
	if path == "" {
		return nil
	}
	_, err = a.library.ScanFolder(a.ctx, path)
	return err
}

// GetLibrarySummary returns the current library, or nil before the first scan.
// The UI calls it on load so a reloaded webview recovers state from Go, which
// stays the source of truth.
func (a *App) GetLibrarySummary() *LibrarySummaryDTO {
	summary, ok := a.library.Summary()
	if !ok {
		return nil
	}
	dto := toLibrarySummaryDTO(summary)
	return &dto
}

// StartServer starts the HTTP server with the default settings. The outcome
// arrives as a "server:state" event; an error here means the request itself was
// refused (for example, the server is already running) or the bind failed.
func (a *App) StartServer() error {
	return a.streaming.Start(a.ctx, streaming.NewConfig())
}

// StopServer stops the HTTP server gracefully.
func (a *App) StopServer() error {
	return a.streaming.Stop(a.ctx)
}

// GetServerState returns the current lifecycle state. The UI calls it on load.
func (a *App) GetServerState() ServerStateDTO {
	return toServerStateDTO(a.streaming.Status())
}

// GetLogHistory returns the recent log lines, oldest first. The UI calls it on
// load, then relies on "log:line" events; Seq lets it join the two without gaps
// or duplicates.
func (a *App) GetLogHistory() []LogLineDTO {
	history := a.logs.History()
	out := make([]LogLineDTO, len(history))
	for i, e := range history {
		out[i] = toLogLineDTO(e)
	}
	return out
}

func openFolderDialog(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", errors.New("app not started")
	}
	return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{
		Title: "Choose Media Folder",
	})
}
