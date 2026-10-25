// EinCast — your home media, on every screen.
//
// main is the composition root: it builds dependencies, hands them to the
// Wails runtime, and contains no logic of its own.
package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"eincast/internal/adapters/fsscan"
	"eincast/internal/adapters/memrepo"
	"eincast/internal/app/library"
	"eincast/internal/bridge"
)

const appName = "EinCast"

// version is stamped at build time: -ldflags "-X main.version=0.1.0".
var version = "dev"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	emitter := bridge.NewEmitter(log)
	libraryService := library.NewService(fsscan.New(log), memrepo.New(), emitter, log)
	app := bridge.New(bridge.Info{Name: appName, Version: version}, libraryService, emitter)

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
