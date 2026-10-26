package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"os"

	"eincast/internal/adapters/safepath"
	"eincast/internal/domain/media"
)

// handleStream serves a file's bytes. http.ServeContent gives us Range
// requests (seeking), HEAD, If-Modified-Since and 206 responses for free.
//
// The URL carries only an ID. The path comes from our own repository, never
// from the request (ADR 0005).
func (a *api) handleStream(w http.ResponseWriter, r *http.Request) {
	it, ok := a.lookup(w, r)
	if !ok {
		return
	}

	realPath, err := safepath.Inside(it.Root, it.Path)
	if err != nil {
		// Possible if a file was swapped for a symlink after the scan. Log the
		// real reason; tell the client nothing beyond "not found".
		a.log.WarnContext(r.Context(), "refusing to serve file",
			errAttr(err), pathAttr(it.Path))
		writeError(w, http.StatusNotFound, codeNotFound, "media not found")
		return
	}

	f, err := os.Open(realPath)
	if err != nil {
		a.fileError(w, r, err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, codeNotFound, "media not found")
		return
	}

	w.Header().Set("Content-Type", media.ContentType(it.Name))
	a.dlnaHeaders(w, r)
	http.ServeContent(w, r, it.Name, info.ModTime(), f)
}

// dlnaHeaders answers a TV that asks for DLNA's content features, which it does
// by sending "getcontentFeatures.dlna.org: 1" with the request. Several TVs
// refuse to play a file whose response lacks them. Only while Smart TV
// discovery is on.
func (a *api) dlnaHeaders(w http.ResponseWriter, r *http.Request) {
	if a.dlna == nil || r.Header.Get("getcontentFeatures.dlna.org") != "1" {
		return
	}
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("contentFeatures.dlna.org", "DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000")
}

func (a *api) fileError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, fs.ErrNotExist) {
		// Deleted since the last scan.
		writeError(w, http.StatusNotFound, codeNotFound, "media not found")
		return
	}
	a.log.ErrorContext(r.Context(), "open media failed", errAttr(err))
	writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
}
