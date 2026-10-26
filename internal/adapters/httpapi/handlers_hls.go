package httpapi

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/transcode"
	"eincast/internal/domain/media"
)

// Transcoding is the slice of the conversion use case the HTTP layer needs.
// Declared here, at the consumer, so the handler is tested against a fake.
type Transcoding interface {
	// Playlist starts converting the video if needed and returns its playlist
	// once playback can begin.
	Playlist(ctx context.Context, id media.ID) (io.ReadSeekCloser, error)
	// File returns a segment of a conversion that is already running.
	File(id media.ID, name string) (io.ReadSeekCloser, error)
	// Available reports whether videos can be converted on this computer.
	Available() bool
}

// handleHLS serves a converted video as an HLS stream: /hls/{id}/index.m3u8 and
// the segments it lists. It exists for devices that cannot play the file as it
// is (an MKV on an iPhone), and is only used when the player page asks for it.
//
// Like every route it takes an ID and a name from a fixed set, never a path
// (ADR 0005), and sits behind the same access control as /stream.
func (a *api) handleHLS(w http.ResponseWriter, r *http.Request) {
	if a.hls == nil {
		writeError(w, http.StatusNotFound, codeNotFound, "conversion is not available")
		return
	}
	id, name := media.ID(r.PathValue("id")), r.PathValue("file")

	var (
		f   io.ReadSeekCloser
		err error
	)
	if name == "index.m3u8" {
		f, err = a.hls.Playlist(r.Context(), id)
	} else {
		f, err = a.hls.File(id, name)
	}
	if err != nil {
		a.hlsError(w, r, err)
		return
	}
	defer f.Close()

	if name == "index.m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		// The playlist grows while the video is converted, so it is never reused.
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "private, max-age=300")
	}
	http.ServeContent(w, r, name, time.Time{}, f)
}

// hlsError answers a failure. Reasons that hold paths or ffmpeg's own words go
// to the log, not to the client.
func (a *api) hlsError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, media.ErrNotFound),
		errors.Is(err, fs.ErrNotExist),
		errors.Is(err, transcode.ErrNotStarted),
		errors.Is(err, app.ErrTranscodingUnavailable):
		writeError(w, http.StatusNotFound, codeNotFound, "no such stream")
	case errors.Is(err, transcode.ErrBusy):
		w.Header().Set("Retry-After", "10")
		writeError(w, http.StatusServiceUnavailable, codeBusy, "other videos are being converted; try again in a moment")
	case errors.Is(err, app.ErrNotConvertible):
		a.log.WarnContext(r.Context(), "could not convert video", errAttr(err))
		writeError(w, http.StatusUnprocessableEntity, codeNotConvert, "this video could not be converted")
	case r.Context().Err() != nil:
		// The client left while waiting; nobody is listening for an answer.
	default:
		a.log.ErrorContext(r.Context(), "conversion failed", errAttr(err))
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
	}
}

// compile-time check that the real service fits the consumer-side interface.
var _ Transcoding = (*transcode.Service)(nil)
