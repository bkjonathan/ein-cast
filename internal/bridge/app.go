// Package bridge is the only package (besides main) that may import the Wails
// runtime. It exposes a small facade to the frontend and translates between
// JS-safe DTOs and the application layer.
package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"eincast/internal/app"
	"eincast/internal/app/settings"
	"eincast/internal/app/streaming"
	"eincast/internal/app/thumbnails"
	"eincast/internal/domain/media"
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
	Add(ctx context.Context, root string) (app.LibraryState, error)
	Remove(id media.LibraryID) error
	Rename(id media.LibraryID, name string) error
	Rescan(ctx context.Context, id media.LibraryID) error
	RescanAll(ctx context.Context) error
	Restore(ctx context.Context) error
	State() []app.LibraryState
	List() []media.Item
	// Watch keeps the library in step with its folder until ctx ends.
	Watch(ctx context.Context)
}

// Settings is the part of the settings use cases the bridge drives.
type Settings interface {
	Get() app.SettingsView
	Update(in settings.Input) (app.SettingsView, error)
	RotateToken() (app.SettingsView, error)
}

// Streaming is the part of the server lifecycle the bridge drives.
type Streaming interface {
	Start(ctx context.Context, cfg streaming.Config) error
	Stop(ctx context.Context) error
	Status() app.ServerStatus
	// Local returns the running server for a tunnel to attach to, or
	// app.ErrServerNotRunning.
	Local() (app.LocalServer, error)
}

// Remote is the part of the remote-access use cases the bridge drives.
type Remote interface {
	Providers() []app.ProviderInfo
	Status() app.RemoteStatus
	Enable(ctx context.Context, id app.ProviderID, srv app.LocalServer) error
	Disable(ctx context.Context) error
	SetPIN(pin string) error
	ClearPIN() error
	ProviderConfig(id app.ProviderID) ([]app.FieldState, error)
	SetProviderField(id app.ProviderID, key, value string) error
	ClearProviderField(id app.ProviderID, key string) error
}

// Thumbnails is the part of the thumbnail use case the bridge needs.
type Thumbnails interface {
	Get(ctx context.Context, id media.ID) (thumbnails.Thumbnail, error)
	Available() bool
}

// Conversions is the part of video conversion the bridge manages: it lives as
// long as the app, and nothing it started may outlive it.
type Conversions interface {
	// Run cleans up idle conversions until ctx ends.
	Run(ctx context.Context)
	// StopAll stops every conversion and deletes its files.
	StopAll()
}

// App is the Wails-bound facade. Every exported method is callable from JS, so
// keep this surface small and stable (and keep lifecycle hooks unexported).
type App struct {
	info      Info
	library   Library
	settings  Settings
	streaming Streaming
	remote    Remote
	thumbs    Thumbnails  // nil: no thumbnails
	convert   Conversions // nil: no conversion
	logs      app.LogStream
	emitter   *Emitter
	log       *slog.Logger

	// ctx lives for the whole app run and is cancelled on shutdown. Wails
	// binding calls carry no context of their own, so long operations use this.
	ctx context.Context
	// pickFolder and pickFile open the native dialogs. Fields so tests can
	// replace the one thing that needs a real window.
	pickFolder func(ctx context.Context) (string, error)
	pickFile   func(ctx context.Context, title string) (string, error)
	// windowThemes maps a theme name to the Wails call that applies it. A field
	// so tests can observe the choice without a native window.
	windowThemes map[string]func(ctx context.Context)
}

// Deps is everything the facade needs, passed in by main. Nothing is global.
type Deps struct {
	Info      Info
	Library   Library
	Settings  Settings
	Streaming Streaming
	Remote    Remote
	// Thumbnails may be nil; the UI then shows icons only.
	Thumbnails Thumbnails
	// Conversions may be nil.
	Conversions Conversions
	Logs        app.LogStream
	Emitter     *Emitter
	Log         *slog.Logger
}

// New builds the facade from its dependencies.
func New(d Deps) *App {
	return &App{
		info:       d.Info,
		library:    d.Library,
		settings:   d.Settings,
		streaming:  d.Streaming,
		remote:     d.Remote,
		thumbs:     d.Thumbnails,
		convert:    d.Conversions,
		logs:       d.Logs,
		emitter:    d.Emitter,
		log:        d.Log,
		pickFolder: openFolderDialog,
		pickFile:   openFileDialog,
		windowThemes: map[string]func(context.Context){
			"system": wruntime.WindowSetSystemDefaultTheme,
			"light":  wruntime.WindowSetLightTheme,
			"dark":   wruntime.WindowSetDarkTheme,
		},
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
	ctx = context.WithoutCancel(ctx)
	// Stopping the server also turns remote access off, but a tunnel must never
	// outlive the app, so do not depend on the server being in a state to stop.
	if err := a.remote.Disable(ctx); err != nil {
		a.log.Warn("turn off remote access on shutdown", slog.String("error", err.Error()))
	}
	if err := a.streaming.Stop(ctx); err != nil && !errors.Is(err, server.ErrInvalidTransition) {
		// Nothing more to do while quitting, but leave a trace.
		a.log.Warn("stop server on shutdown", slog.String("error", err.Error()))
	}
	// No ffmpeg or temporary file may outlive the app.
	if a.convert != nil {
		a.convert.StopAll()
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emitter.bind(ctx)
	// Owned by the app context: the goroutine ends, and unsubscribes, when the
	// app shuts down.
	go a.forwardLogs(ctx)
	// Follows the library folder on disk; also ends with the app context.
	go a.library.Watch(ctx)
	if a.convert != nil {
		go a.convert.Run(ctx)
	}
	// Reopen the last folder in the background so the window appears at once.
	go a.restoreLibrary(ctx)
}

// restoreLibrary rescans the folder remembered from the last run. A failure is
// not fatal: the reason is kept in the library status and shown on the first-run
// screen.
func (a *App) restoreLibrary(ctx context.Context) {
	if err := a.library.Restore(ctx); err != nil {
		a.log.Warn("could not reopen the previous library folder", slog.String("error", err.Error()))
	}
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

// ChooseFolder opens the native folder picker and adds the chosen folder as a
// library, scanning it. The result arrives as "library:status" and
// "library:scanned" events; this method only reports failure (a folder that is
// already in the library, one that cannot be read). Cancelling the dialog is not
// an error.
func (a *App) ChooseFolder() error {
	path, err := a.pickFolder(a.ctx)
	if err != nil {
		return fmt.Errorf("choose folder: %w", err)
	}
	if path == "" {
		return nil
	}
	_, err = a.library.Add(a.ctx, path)
	return err
}

// ListLibraries returns every library with what is in it and what is happening
// to it. The UI calls it on load so a reloaded webview recovers state from Go,
// which stays the source of truth.
func (a *App) ListLibraries() []LibraryStateDTO {
	return toLibraryStateDTOs(a.library.State())
}

// RemoveLibrary takes a library out of the list. Its folder on disk is untouched.
func (a *App) RemoveLibrary(id string) error {
	return a.library.Remove(media.LibraryID(id))
}

// RenameLibrary changes the name a library is shown as.
func (a *App) RenameLibrary(id, name string) error {
	return a.library.Rename(media.LibraryID(id), name)
}

// RescanLibrary scans one library again. The result arrives as events.
func (a *App) RescanLibrary(id string) error {
	return a.library.Rescan(a.ctx, media.LibraryID(id))
}

// RescanAllLibraries scans every library again.
func (a *App) RescanAllLibraries() error {
	return a.library.RescanAll(a.ctx)
}

// ListLibrary returns every item in the library, in scan order. The UI calls it
// on load and again after each "library:scanned" event. Disk paths stay in Go:
// the DTO carries the ID and the path relative to the library folder.
func (a *App) ListLibrary() []LibraryItemDTO {
	return toLibraryItemDTOs(a.library.List())
}

// GetThumbnail returns a small picture of an item as a "data:image/jpeg;base64,…"
// URL, ready for an <img>. It returns "" when the item has no picture (a song
// without cover art), when ffmpeg is not installed, or when the ID is unknown:
// the UI shows an icon in all those cases, so none of them is an error. The
// first request for an item can take a moment while the picture is made.
func (a *App) GetThumbnail(id string) (string, error) {
	if a.thumbs == nil {
		return "", nil
	}
	th, err := a.thumbs.Get(a.ctx, media.ID(id))
	switch {
	case err == nil:
		return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(th.JPEG), nil
	case errors.Is(err, media.ErrNotFound),
		errors.Is(err, app.ErrNoThumbnail),
		errors.Is(err, app.ErrThumbnailsUnavailable):
		return "", nil
	default:
		return "", fmt.Errorf("thumbnail: %w", err)
	}
}

// ThumbnailsAvailable says whether thumbnails can be made on this computer, so
// the Library screen can suggest installing ffmpeg when they cannot.
func (a *App) ThumbnailsAvailable() bool {
	return a.thumbs != nil && a.thumbs.Available()
}

// SetWindowTheme asks the native window to use "system", "light" or "dark"
// chrome. Today only Windows acts on it (Mica follows the choice); on macOS
// Wails ignores the call, and the page copes with a tint instead. The theme of
// the page itself is the frontend's business.
func (a *App) SetWindowTheme(mode string) error {
	apply, ok := a.windowThemes[mode]
	if !ok {
		return fmt.Errorf("set window theme: unknown theme %q", mode)
	}
	apply(a.ctx)
	return nil
}

// StartServer starts the HTTP server with the saved port and name. The outcome
// arrives as a "server:state" event; an error here means the request itself was
// refused (for example, the server is already running) or the bind failed.
func (a *App) StartServer() error {
	s := a.settings.Get()
	return a.streaming.Start(a.ctx, streaming.NewConfig(
		streaming.WithPort(s.Port),
		streaming.WithName(s.Name),
		// Never together with the token (settings refuses that too): a TV cannot
		// present it, so this is a second lock on the same door.
		streaming.WithDLNA(s.DLNA && !s.RequireToken),
	))
}

// StopServer stops the HTTP server gracefully.
func (a *App) StopServer() error {
	return a.streaming.Stop(a.ctx)
}

// GetServerState returns the current lifecycle state. The UI calls it on load.
func (a *App) GetServerState() ServerStateDTO {
	return toServerStateDTO(a.streaming.Status())
}

// GetSettings returns the saved settings and, when switched on, the access token.
func (a *App) GetSettings() SettingsDTO {
	return toSettingsDTO(a.settings.Get())
}

// UpdateSettings validates and saves the settings. The new values arrive as a
// "settings:changed" event; a rejected value is returned as an error. A new
// port, name or Smart TV setting applies the next time the server starts; the
// token applies at once.
func (a *App) UpdateSettings(in SettingsInputDTO) error {
	_, err := a.settings.Update(settings.Input{Port: in.Port, Name: in.Name, RequireToken: in.RequireToken, DLNA: in.DLNA})
	return err
}

// RotateAccessToken replaces the access token. Devices using the old link are
// locked out until they use the new one.
func (a *App) RotateAccessToken() error {
	_, err := a.settings.RotateToken()
	return err
}

// GetRemoteStatus returns remote access as it is now. The UI calls it on load.
func (a *App) GetRemoteStatus() RemoteStatusDTO {
	return toRemoteStatusDTO(a.remote.Status())
}

// ListRemoteProviders returns the ways of reaching the library from outside.
func (a *App) ListRemoteProviders() []RemoteProviderDTO {
	return toRemoteProviderDTOs(a.remote.Providers())
}

// EnableRemote turns on a remote access provider for the running server. The
// outcome arrives as "remote:status" events; an error here means the request
// was refused (no PIN for a public provider, server not running) or the
// provider could not start. It can take a while, so the UI shows it as pending.
func (a *App) EnableRemote(id string) error {
	srv, err := a.streaming.Local()
	if err != nil {
		return remoteError(err)
	}
	return remoteError(a.remote.Enable(a.ctx, app.ProviderID(id), srv))
}

// DisableRemote turns remote access off and signs everyone out.
func (a *App) DisableRemote() error {
	return remoteError(a.remote.Disable(a.ctx))
}

// SetRemotePIN sets or changes the access PIN that public access requires.
// Changing it signs out every remote browser.
func (a *App) SetRemotePIN(pin string) error {
	return remoteError(a.remote.SetPIN(pin))
}

// ClearRemotePIN removes the access PIN. It is refused while public access is on.
func (a *App) ClearRemotePIN() error {
	return remoteError(a.remote.ClearPIN())
}

// GetRemoteProviderConfig returns which of a provider's settings are saved.
// Secrets are reported as set or not; their values never leave Go.
func (a *App) GetRemoteProviderConfig(id string) ([]RemoteFieldStateDTO, error) {
	states, err := a.remote.ProviderConfig(app.ProviderID(id))
	if err != nil {
		return nil, remoteError(err)
	}
	return toRemoteFieldStateDTOs(states), nil
}

// SetRemoteProviderField saves one setting of a provider (a token, a host name).
// Refused while that provider is on. The new state arrives as a "remote:status"
// event.
func (a *App) SetRemoteProviderField(id, key, value string) error {
	return remoteError(a.remote.SetProviderField(app.ProviderID(id), key, value))
}

// ClearRemoteProviderField removes one setting of a provider.
func (a *App) ClearRemoteProviderField(id, key string) error {
	return remoteError(a.remote.ClearProviderField(app.ProviderID(id), key))
}

// ChooseFile opens the native file picker and returns the chosen path, or "" if
// the person cancelled. Used for settings that name a program on disk.
func (a *App) ChooseFile(title string) (string, error) {
	path, err := a.pickFile(a.ctx, title)
	if err != nil {
		return "", fmt.Errorf("choose file: %w", err)
	}
	return path, nil
}

// remoteError rewrites the refusals a person can act on as plain advice and
// passes everything else through. These messages are shown in the UI.
func remoteError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, app.ErrPINRequired):
		return errors.New("Set an access PIN before turning on public access.")
	case errors.Is(err, app.ErrPublicActive):
		return errors.New("Turn off public access before removing the PIN.")
	case errors.Is(err, app.ErrAlreadyEnabled):
		return errors.New("Remote access is already on.")
	case errors.Is(err, app.ErrServerNotRunning):
		return errors.New("Start the server first.")
	case errors.Is(err, app.ErrUnknownProvider):
		return errors.New("That way of connecting is not available.")
	case errors.Is(err, app.ErrUnknownField):
		return errors.New("That setting does not exist.")
	case errors.Is(err, app.ErrProviderInUse):
		return errors.New("Turn this option off before changing its settings.")
	case errors.Is(err, app.ErrNotConfigured):
		var nc *app.NotConfiguredError
		if errors.As(err, &nc) {
			return errors.New("Finish setting this up first: " + strings.Join(nc.Missing, ", ") + ".")
		}
		return errors.New("Finish setting this up first.")
	case errors.Is(err, app.ErrInvalidValue):
		// "Hostname: invalid value: enter the hostname only…" reads better without the middle.
		return errors.New(strings.Replace(err.Error(), "invalid value: ", "", 1))
	default:
		return err
	}
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

func openFileDialog(ctx context.Context, title string) (string, error) {
	if ctx == nil {
		return "", errors.New("app not started")
	}
	return wruntime.OpenFileDialog(ctx, wruntime.OpenDialogOptions{Title: title})
}
