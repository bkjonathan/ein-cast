// Package bridge is the only package (besides main) that may import the Wails
// runtime. It exposes a small facade to the frontend and translates between
// JS-safe DTOs and the application layer.
package bridge

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"eincast/internal/app"
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

// App is the Wails-bound facade. Every exported method is callable from JS, so
// keep this surface small and stable (and keep lifecycle hooks unexported).
type App struct {
	info    Info
	library Library
	emitter *Emitter

	// ctx lives for the whole app run and is cancelled on shutdown. Wails
	// binding calls carry no context of their own, so long operations use this.
	ctx context.Context
	// pickFolder opens the native directory dialog. A field so tests can
	// replace the one thing that needs a real window.
	pickFolder func(ctx context.Context) (string, error)
}

// New builds the facade. Dependencies are passed in; nothing is global.
func New(info Info, library Library, emitter *Emitter) *App {
	return &App{
		info:       info,
		library:    library,
		emitter:    emitter,
		pickFolder: openFolderDialog,
	}
}

// OnStartup returns the hook to give to wails.Options.OnStartup. It is a
// function rather than an exported method because exported methods are bound
// to JS.
func OnStartup(a *App) func(ctx context.Context) {
	return a.startup
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emitter.bind(ctx)
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

func openFolderDialog(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", errors.New("app not started")
	}
	return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{
		Title: "Choose Media Folder",
	})
}
