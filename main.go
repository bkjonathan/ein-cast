// EinCast — your home media, on every screen.
//
// main is the composition root: it builds dependencies, hands them to the
// Wails runtime, and contains no logic of its own.
package main

import (
	"embed"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"eincast/internal/adapters/fsscan"
	"eincast/internal/adapters/httpapi"
	"eincast/internal/adapters/logbus"
	"eincast/internal/adapters/memrepo"
	"eincast/internal/adapters/netinfo"
	"eincast/internal/adapters/secrets"
	"eincast/internal/adapters/settingsfile"
	"eincast/internal/app/library"
	"eincast/internal/app/settings"
	"eincast/internal/app/streaming"
	"eincast/internal/bridge"
)

const appName = "EinCast"

// version is stamped at build time: -ldflags "-X main.version=0.1.0".
var version = "dev"

// logHistory is how many recent log lines are kept for a UI that opens late.
const logHistory = 2000

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Logs go to the console as before and, through the bus, to the UI.
	logs := logbus.NewBus(logHistory)
	log := slog.New(logbus.NewHandler(slog.NewTextHandler(os.Stderr, nil), logs))

	emitter := bridge.NewEmitter(log)

	// Settings live in the per-user config directory; the access token lives in
	// the OS credential store, never in that file.
	settingsService := settings.NewService(settingsfile.New(settingsDir(log)), secrets.New(), emitter, log)
	settingsService.Load()

	libraryService := library.NewService(fsscan.New(log), memrepo.New(), settingsService, emitter, log)
	webServer := httpapi.New(libraryService, settingsService, version, log)
	streamingService := streaming.NewService(webServer, netinfo.New(), emitter, log)
	app := bridge.New(bridge.Deps{
		Info:      bridge.Info{Name: appName, Version: version},
		Library:   libraryService,
		Settings:  settingsService,
		Streaming: streamingService,
		Logs:      logs,
		Emitter:   emitter,
		Log:       log,
	})

	err := wails.Run(&options.App{
		Title:     appName,
		Width:     1100,
		Height:    720,
		MinWidth:  880,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Fully transparent so the native vibrancy / Mica backdrop shows through
		// the glass panels.
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		OnStartup:        bridge.OnStartup(app),
		OnShutdown:       bridge.OnShutdown(app),
		Bind:             []any{app},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   appName + " " + version,
				Message: "Your home media, on every screen.",
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
