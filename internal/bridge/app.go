// Package bridge is the only package (besides main) that may import the Wails
// runtime. It exposes a small facade to the frontend and translates between
// JS-safe DTOs and the application layer.
package bridge

import "runtime"

// Info is the static identity of the running build, supplied by main.
type Info struct {
	Name    string
	Version string
}

// App is the Wails-bound facade. Every exported method is callable from JS,
// so keep this surface small and stable.
type App struct {
	info Info
}

// New builds the facade. Dependencies are passed in; nothing is global.
func New(info Info) *App {
	return &App{info: info}
}

// AppInfo returns the app name, version and host platform.
func (a *App) AppInfo() AppInfoDTO {
	return AppInfoDTO{
		Name:     a.info.Name,
		Version:  a.info.Version,
		Platform: runtime.GOOS,
	}
}
