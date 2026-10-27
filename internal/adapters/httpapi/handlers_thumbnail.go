package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"

	"eincast/internal/app"
	"eincast/internal/app/thumbnails"
	"eincast/internal/domain/media"
)

// Thumbnails is the slice of the thumbnail use case the HTTP layer needs.
// Declared here, at the consumer, so the handler is tested against a fake.
type Thumbnails interface {
	Get(ctx context.Context, id media.ID) (thumbnails.Thumbnail, error)
	// Available reports whether new thumbnails can be made on this computer.
	Available() bool
}

// handleThumbnail serves a small JPEG of an item: a video frame, a song's cover,
// a shrunken photo. Like /stream it takes an ID, never a path (ADR 0005), and
// sits behind the same access control.
//
// "No picture" is a plain 404: a client shows an icon instead, and nothing is
// logged per request since many files simply have none.
func (a *api) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	if a.thumbs == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "thumbnails are not available")
		return
	}
	it, ok := a.lookup(w, r) // the profile check, before anything is made
	if !ok {
		return
	}
	th, err := a.thumbs.Get(r.Context(), it.ID)
	switch {
	case err == nil:
	case errors.Is(err, media.ErrNotFound),
		errors.Is(err, app.ErrNoThumbnail),
		errors.Is(err, app.ErrThumbnailsUnavailable):
		writeError(w, http.StatusNotFound, codeNotFound, "no thumbnail")
		return
	case r.Context().Err() != nil:
		return // the client left; nobody is listening for an answer
	default:
		a.log.ErrorContext(r.Context(), "thumbnail failed", errAttr(err))
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	// private: it sits behind a sign-in, so shared caches must not keep it. The
	// picture changes only when the file does, and ServeContent adds
	// Last-Modified from the file's own time, so revalidation is cheap.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", th.ModTime, bytes.NewReader(th.JPEG))
}
