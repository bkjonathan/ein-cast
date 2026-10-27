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
	// Title is what to show for the item: read from the file name or a .nfo, or
	// the file name itself. Name stays the file name.
	Title string `json:"title"`
	// Year is 0 when unknown. Show, Season and Episode are set for an episode of
	// a series (Show empty, numbers 0 otherwise). Plot comes from a .nfo only.
	Year    int    `json:"year"`
	Show    string `json:"show"`
	Season  int    `json:"season"`
	Episode int    `json:"episode"`
	Plot    string `json:"plot"`
	// LegacyID is the ID the item had before libraries existed. The player page
	// moves a resume position saved under it to the current ID.
	LegacyID media.ID `json:"legacyId"`
	// Poster says /thumb/{id} is a poster found next to the file (a portrait
	// picture), not a frame taken from it.
	Poster bool `json:"poster"`
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
	m := it.Meta
	show, season, episode := "", 0, 0
	if m.IsEpisode() {
		show, season, episode = m.Show, m.Season, m.Episode
	}
	return itemJSON{
		ID: it.ID, Name: it.Name, Kind: it.Kind, Size: it.Size, ModTime: it.ModTime, Subtitles: subs,
		Title: it.DisplayName(), Year: m.Year, Show: show, Season: season, Episode: episode, Plot: m.Plot,
		Poster: it.Artwork.Present(), LegacyID: media.LegacyID(it.RelPath),
	}
}

// handleLibrary lists the items the request may see.
func (a *api) handleLibrary(w http.ResponseWriter, r *http.Request) {
	items, _, ok := a.visibleItems(w, r)
	if !ok {
		return
	}
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
// response and ok is false. An item the request may not see is the same 404 as one
// that does not exist: the answer must not confirm that an ID is real (ADR 0027).
// Every route that takes an ID goes through here.
func (a *api) lookup(w http.ResponseWriter, r *http.Request) (media.Item, bool) {
	it, err := a.lib.Get(media.ID(r.PathValue("id")))
	switch {
	case err == nil:
		if !a.allowed(r, it) {
			writeError(w, http.StatusNotFound, codeNotFound, "media not found")
			return media.Item{}, false
		}
		return it, true
	case errors.Is(err, media.ErrNotFound):
		writeError(w, http.StatusNotFound, codeNotFound, "media not found")
	default:
		a.log.ErrorContext(r.Context(), "library lookup failed", errAttr(err))
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
	}
	return media.Item{}, false
}
