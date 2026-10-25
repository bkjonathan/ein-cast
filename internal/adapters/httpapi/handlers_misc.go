package httpapi

import (
	"log/slog"
	"net/http"
)

type healthJSON struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Name    string `json:"name"`
}

func (a *api) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthJSON{Status: "ok", Version: a.version, Name: a.name})
}

// indexHTML is a stand-in landing page; the embedded player replaces it in the
// Connect phase.
const indexHTML = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>EinCast</title>
<body style="font-family:system-ui;max-width:32rem;margin:3rem auto;padding:0 1rem">
<h1>EinCast is running</h1>
<p>Open the <a href="/playlist.m3u">playlist</a> in VLC or Infuse, or browse the
<a href="/api/library">library JSON</a>.</p>
`

func (a *api) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

// handleNotFound keeps even unknown routes in the API's JSON error shape.
func (a *api) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, codeNotFound, "no such route")
}

func errAttr(err error) slog.Attr { return slog.String("error", err.Error()) }

func pathAttr(p string) slog.Attr { return slog.String("path", p) }
