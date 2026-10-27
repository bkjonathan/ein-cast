// EinCast — your home media, on every screen.
//
// main is the composition root: it builds dependencies, hands them to the
// Wails runtime, and contains no logic of its own.
package main

import (
	"context"
	"embed"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"eincast/internal/adapters/dlna"
	"eincast/internal/adapters/eraser"
	"eincast/internal/adapters/ffmpeg"
	"eincast/internal/adapters/fsscan"
	"eincast/internal/adapters/fswatch"
	"eincast/internal/adapters/githubrelease"
	"eincast/internal/adapters/httpapi"
	"eincast/internal/adapters/logbus"
	"eincast/internal/adapters/loginitem"
	"eincast/internal/adapters/mdns"
	"eincast/internal/adapters/memrepo"
	"eincast/internal/adapters/netinfo"
	"eincast/internal/adapters/portproc"
	"eincast/internal/adapters/powerguard"
	"eincast/internal/adapters/profilefile"
	"eincast/internal/adapters/relaunch"
	"eincast/internal/adapters/resumefile"
	"eincast/internal/adapters/secrets"
	"eincast/internal/adapters/settingsfile"
	"eincast/internal/adapters/thumbcache"
	"eincast/internal/adapters/tray"
	"eincast/internal/adapters/tunnel"
	"eincast/internal/adapters/tunnel/cloudflared"
	"eincast/internal/adapters/tunnel/ngrok"
	"eincast/internal/adapters/tunnel/tailscale"
	"eincast/internal/app"
	"eincast/internal/app/diagnostics"
	"eincast/internal/app/library"
	"eincast/internal/app/portfree"
	"eincast/internal/app/profiles"
	"eincast/internal/app/providerconfig"
	"eincast/internal/app/remote"
	"eincast/internal/app/reset"
	"eincast/internal/app/resume"
	"eincast/internal/app/settings"
	"eincast/internal/app/streaming"
	"eincast/internal/app/thumbnails"
	"eincast/internal/app/tracks"
	"eincast/internal/app/transcode"
	"eincast/internal/app/updates"
	"eincast/internal/bridge"
)

const appName = "EinCast"

// version is stamped at build time: -ldflags "-X main.version=1.0.1".
var version = "dev"

// releaseRepo is where releases are published; the update check reads it.
const releaseRepo = "bkjonathan/ein-cast"

// thumbnailMaxAge is how long an unrefreshed thumbnail is kept. A changed file
// gets a new one, so the old picture would otherwise stay forever.
const thumbnailMaxAge = 90 * 24 * time.Hour

// logHistory is how many recent log lines are kept for a UI that opens late.
const logHistory = 2000

//go:embed all:frontend/dist
var assets embed.FS

// appIcon is shown in the macOS About panel; the same file builds the Dock icon.
//
//go:embed build/appicon.png
var appIcon []byte

// trayIcon is the Windows notification-area icon; it must be an .ico.
//
//go:embed build/windows/icon.ico
var trayIcon []byte

// singleInstanceID names the lock that keeps a second EinCast from starting: it
// would fight the first over the port. Launching again brings the first window
// back instead.
const singleInstanceID = "com.bkthomas.eincast"

func main() {
	// Logs go to the console as before and, through the bus, to the UI.
	logs := logbus.NewBus(logHistory)
	log := slog.New(logbus.NewHandler(slog.NewTextHandler(os.Stderr, nil), logs))

	// A copy opened by "Reset EinCast" waits here for the old one to be gone, so
	// it never reads files the old one is still deleting, nor meets its lock.
	relaunch.WaitIfRelaunched(os.Args[1:], log)

	emitter := bridge.NewEmitter(log)

	// Settings live in the per-user config directory; the access token and the
	// remote-access PIN live in the OS credential store, never in that file.
	credentialStore := secrets.New()
	//
	// Closing the window only hides the app when something can bring it back:
	// the Dock on macOS. Windows has the tray icon too, but it has not been run
	// on a real Windows machine yet, so a new install there still quits on close
	// (ADR 0030). Whoever turns it on in Settings gets it.
	settingsService := settings.NewService(settingsfile.New(settingsDir(log)), credentialStore, emitter, log,
		settings.WithHideOnCloseByDefault(runtime.GOOS == "darwin"),
		settings.WithLoginItem(loginItem(log)))
	settingsService.Load()
	startup := settingsService.Get()

	// Remote access. Providers arrive one per step of the remote-access phase
	// (docs/ROADMAP.md). The recommended one is listed first, then the ones that
	// need the least setup, then the ones that need the most.
	providerSettings := providerconfig.New(credentialStore)
	providers, err := tunnel.NewRegistry(
		tailscale.New(log),
		cloudflared.NewQuick(providerSettings, log),
		cloudflared.NewNamed(providerSettings, log),
		ngrok.New(providerSettings, log),
	)
	if err != nil {
		log.Error("tunnel providers", slog.String("error", err.Error()))
		os.Exit(1)
	}
	remoteService := remote.NewService(providers, credentialStore, emitter, log)
	remoteService.Load()

	libraryService := library.NewService(fsscan.New(log), memrepo.New(), settingsService, emitter, log,
		library.WithWatcher(fswatch.New(log), library.DefaultWatchTiming))
	// Who is watching: profiles (with their PIN hashes) and where each of them
	// stopped live in files beside settings.json, and a profile's positions go with it.
	configDir := settingsDir(log)
	resumeService := resume.NewService(resumefile.New(configDir), log)
	resumeService.Load()
	profileService := profiles.NewService(profilefile.New(configDir), credentialStore, libraryService, emitter, log,
		profiles.WithOnDelete(resumeService.Forget))
	profileService.Load()

	// Thumbnails are made on demand with the ffmpeg already on the computer and
	// kept in the per-user cache folder, so they are made once.
	thumbCache := thumbcache.New(cacheDir(log, "thumbs"), log)
	thumbCache.Prune(thumbnailMaxAge)
	// Where ffmpeg is can be chosen in Settings; both users of it read the choice
	// each time, so a change applies without a restart.
	chosenFFmpeg := ffmpeg.WithChosenPath(func() string { return settingsService.Get().FFmpegPath })
	thumbnailer := ffmpeg.New(log, chosenFFmpeg)
	thumbnailService := thumbnails.NewService(libraryService, thumbnailer, thumbCache, log, thumbnails.DefaultOptions)

	// Videos a device cannot play as they are are converted to HLS by the same
	// ffmpeg, into temporary folders that are emptied at every start and stopped
	// with the app.
	// The video chip is used when the person allows it (Settings, on by default);
	// a chip that does not work is given up on for that video by itself.
	hardwareEncoding := ffmpeg.WithHardwareEncoding(func() bool { return settingsService.Get().HardwareEncoding })
	conversions := transcode.NewService(libraryService,
		ffmpeg.NewTranscoder(cacheDir(log, "hls"), log, chosenFFmpeg, hardwareEncoding), log, transcode.DefaultOptions)

	// The audio and subtitle tracks inside a video are read with the same
	// programs. An extracted subtitle is kept like a thumbnail, since finding it
	// means reading the whole file.
	subtitleCache := thumbcache.New(cacheDir(log, "subtitles"), log, thumbcache.WithFiles("subtitle", ".vtt"))
	subtitleCache.Prune(thumbnailMaxAge)
	trackService := tracks.NewService(libraryService, ffmpeg.NewProber(log, chosenFFmpeg),
		ffmpeg.NewExtractor(log, chosenFFmpeg), subtitleCache, log, tracks.DefaultOptions)

	// Looking for a newer version is the one network call EinCast makes by itself.
	// It only reads, can be turned off in Settings, and the person installs the
	// update (ADR 0029).
	updateService := updates.NewService(githubrelease.New(releaseRepo), settingsService, emitter, log,
		version, updates.DefaultTiming, updates.WithPlatform(runtime.GOOS))

	// Smart TVs identify a server by a UUID; deriving it from the computer's name
	// keeps it the same on every run without storing anything.
	hostname, err := os.Hostname()
	if err != nil {
		hostname = appName
	}
	deviceID := dlna.DeviceUUID(hostname)

	webServer := httpapi.New(libraryService, settingsService, remoteService, version, log,
		httpapi.WithThumbnails(thumbnailService), httpapi.WithTranscoding(conversions), httpapi.WithTracks(trackService),
		httpapi.WithDLNA(deviceID),
		httpapi.WithProfiles(profileService), httpapi.WithResume(resumeService))
	// The server withdraws public access before it stops, so a tunnel never
	// outlives the server it points at.
	streamingService := streaming.NewService(webServer, netinfo.New(), emitter, log,
		streaming.WithPublicAccess(remoteService),
		// A sleeping laptop is a server nobody can reach; held only while it runs.
		streaming.WithPowerGuard(powerguard.New(log)),
		// Once the server runs, make it findable: by name for computers and phones,
		// and, only when the person turned it on, by TVs.
		streaming.WithDiscovery(mdns.New(log), dlna.NewAnnouncer(deviceID, version, log)))
	// A port some other program is holding can be freed from Settings. The
	// service needs our own process id to refuse ending ourselves.
	portService := portfree.NewService(portproc.New(), os.Getpid(), portfree.DefaultTiming, log)

	// "Reset EinCast" in Settings: everything EinCast made for itself goes (its
	// settings folder, its cache folder, its entries in the credential store, the
	// open-at-login entry), and a new copy opens as a first run. The media folders
	// are not on this list (ADR 0034).
	resetService := reset.NewService(eraser.New(settingsDir(log), cacheDir(log, "")), credentialStore,
		loginItem(log), relaunch.New(log), log)

	// "Copy diagnostics" in Settings: what to paste into a bug report, made only
	// when asked for and with folder names and secrets left out.
	home, _ := os.UserHomeDir() // empty when unknown: nothing then to hide
	diagnosticsService := diagnostics.NewService(
		diagnostics.Identity{Version: version, OS: runtime.GOOS, Arch: runtime.GOARCH, Home: home},
		diagnostics.Deps{Settings: settingsService, Server: streamingService, Remote: remoteService,
			Libraries: libraryService, Logs: logs, FFmpeg: thumbnailer})

	app := bridge.New(bridge.Deps{
		Info:        bridge.Info{Name: appName, Version: version, Repo: releaseRepo},
		Library:     libraryService,
		Settings:    settingsService,
		Streaming:   streamingService,
		Remote:      remoteService,
		Ports:       portService,
		Thumbnails:  thumbnailService,
		Conversions: conversions,
		FFmpeg:      thumbnailer,
		Profiles:    profileService,
		Updates:     updateService,
		Reset:       resetService,
		Diagnostics: diagnosticsService,
		// The last seconds of watching are written once the server has stopped.
		OnShutdown: []func(){func() {
			if err := resumeService.Close(); err != nil {
				log.Warn("could not save resume positions", slog.String("error", err.Error()))
			}
		}},
		Logs:    logs,
		Emitter: emitter,
		Log:     log,
	})

	// Signing in launches EinCast with this flag; it then comes up without a
	// window in the way, but only where the window can be brought back.
	startHidden := slices.Contains(os.Args[1:], loginitem.BackgroundFlag) && startup.HideOnClose

	err = wails.Run(&options.App{
		Title:             appName,
		Width:             1100,
		Height:            720,
		MinWidth:          880,
		MinHeight:         560,
		HideWindowOnClose: startup.HideOnClose,
		StartHidden:       startHidden,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               singleInstanceID,
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { bridge.ShowWindow(app) },
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Fully transparent so the native vibrancy / Mica backdrop shows through
		// the glass panels.
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		OnStartup: func(ctx context.Context) {
			bridge.OnStartup(app)(ctx)
			if startup.HideOnClose {
				// Nothing on macOS (the Dock does this job); on Windows the icon
				// that brings the window back.
				tray.Run(ctx, log, trayIcon, appName,
					func() { bridge.ShowWindow(app) }, func() { bridge.Quit(app) })
			}
		},
		OnShutdown: bridge.OnShutdown(app),
		Bind:       []any{app},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   appName + " " + version,
				Message: "Your home media, on every screen.",
				Icon:    appIcon,
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         windows.Mica,
		},
	})
	if err != nil {
		log.Error("wails run", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// loginItem is the way to open EinCast at login on this system, or nil when
// there is none, in which case the setting is accepted and does nothing.
func loginItem(log *slog.Logger) app.LoginItem {
	item, err := loginitem.New(log)
	if err != nil {
		log.Info("cannot open at login on this system", slog.String("reason", err.Error()))
		return nil
	}
	return item
}

// settingsDir is where settings.json lives: <user config dir>/EinCast. If the
// platform cannot name one, fall back to the temp directory so the app still
// runs; the settings simply may not survive a reboot.
func settingsDir(log *slog.Logger) string {
	base, err := os.UserConfigDir()
	if err != nil {
		log.Warn("no user config directory; settings will be kept in the temp directory",
			slog.String("error", err.Error()))
		base = os.TempDir()
	}
	return filepath.Join(base, appName)
}

// cacheDir is where regenerable files live: <user cache dir>/EinCast/<name>.
// The operating system may clear it at any time, which only costs making things
// again. With no cache directory it falls back to the temp directory.
func cacheDir(log *slog.Logger, name string) string {
	base, err := os.UserCacheDir()
	if err != nil {
		log.Warn("no user cache directory; cached files will be kept in the temp directory",
			slog.String("error", err.Error()))
		base = os.TempDir()
	}
	return filepath.Join(base, appName, name)
}
