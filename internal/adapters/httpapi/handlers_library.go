package httpapi

import (
	"errors"
	"net/http"
	"time"

	"eincast/internal/domain/media"
)

// itemJSON is the API's own view of a media item. It is separate from
// media.Item on purpose: the absolute path must never be serialized, and the
// wire format should not change when the domain type does.
type itemJSON struct {
	ID      media.ID   `json:"id"`
	Name    string     `json:"name"`
	Kind    media.Kind `json:"kind"`
	Size    int64      `json:"size"`
	ModTime time.Time  `json:"modTime"`
	// Subtitles lists the tracks of a video, in order. A track is fetched as
	// /subtitle/{id}/{position}; its file name and disk path are not exposed.
	Subtitles []subtitleJSON `json:"subtitles"`
}

type subtitleJSON struct {
	Label string `json:"label"`
	Lang  string `json:"lang"`
}

func toItemJSON(it media.Item) itemJSON {
	subs := make([]subtitleJSON, len(it.Subtitles)) // non-nil: [] rather than null
	for i, s := range it.Subtitles {
		subs[i] = subtitleJSON{Label: s.Label, Lang: s.Lang}
	}
	return itemJSON{ID: it.ID, Name: it.Name, Kind: it.Kind, Size: it.Size, ModTime: it.ModTime, Subtitles: subs}
}

// handleLibrary lists every item.
func (a *api) handleLibrary(w http.ResponseWriter, _ *http.Request) {
	items := a.lib.List()
	out := make([]itemJSON, 0, len(items)) // non-nil: an empty library is [], not null
	for _, it := range items {
		out = append(out, toItemJSON(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleLibraryItem returns one item.
func (a *api) handleLibraryItem(w http.ResponseWriter, r *http.Request) {
	it, ok := a.lookup(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toItemJSON(it))
}

// lookup resolves the {id} path value. On failure it has already written the
// response and ok is false.
func (a *api) lookup(w http.ResponseWriter, r *http.Request) (media.Item, bool) {
	it, err := a.lib.Get(media.ID(r.PathValue("id")))
	switch {
	case err == nil:
		return it, true
	case errors.Is(err, media.ErrNotFound):
		writeError(w, http.StatusNotFound, codeNotFound, "media not found")
	default:
		a.log.ErrorContext(r.Context(), "library lookup failed", errAttr(err))
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
	}
	return media.Item{}, false
}
