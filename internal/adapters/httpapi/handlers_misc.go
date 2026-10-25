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

// handleNotFound keeps even unknown routes in the API's JSON error shape.
func (a *api) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, codeNotFound, "no such route")
}

func errAttr(err error) slog.Attr { return slog.String("error", err.Error()) }

func pathAttr(p string) slog.Attr { return slog.String("path", p) }
