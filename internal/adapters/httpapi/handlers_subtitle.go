package httpapi

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"

	"eincast/internal/adapters/safepath"
	"eincast/internal/domain/subtitle"
)

// maxSubtitleBytes bounds what is read for one track. Real subtitle files are
// tens to hundreds of kilobytes; this stops a mislabelled huge file from being
// loaded into memory.
const maxSubtitleBytes = 4 << 20

// handleSubtitle serves one subtitle track of a video as WebVTT, converted on the
// way out so the file on disk is never touched. The URL is the video's ID and
// the track's position in its list (see itemJSON); like every other route it
// never carries a path (ADR 0005).
func (a *api) handleSubtitle(w http.ResponseWriter, r *http.Request) {
	it, ok := a.lookup(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("track"))
	if err != nil || n < 0 || n >= len(it.Subtitles) {
		writeError(w, http.StatusNotFound, codeNotFound, "no such subtitle")
		return
	}

	realPath, err := safepath.Inside(it.Root, it.Subtitles[n].Path)
	if err != nil {
		a.log.WarnContext(r.Context(), "refusing to serve subtitle", errAttr(err), pathAttr(it.Subtitles[n].Path))
		writeError(w, http.StatusNotFound, codeNotFound, "no such subtitle")
		return
	}
	raw, err := readLimited(realPath, maxSubtitleBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errTooLarge) {
			writeError(w, http.StatusNotFound, codeNotFound, "no such subtitle")
			return
		}
		a.log.ErrorContext(r.Context(), "read subtitle failed", errAttr(err))
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
		return
	}

	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	_, _ = w.Write([]byte(subtitle.ToWebVTT(subtitle.Decode(raw))))
}

var errTooLarge = errors.New("file is too large")

// readLimited reads a whole file, refusing one longer than max bytes.
func readLimited(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errTooLarge
	}
	return data, nil
}
