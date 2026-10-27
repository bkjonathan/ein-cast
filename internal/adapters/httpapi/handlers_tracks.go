package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// Tracks is the slice of the track use case the HTTP layer needs: what is inside
// a video, and its embedded subtitles as WebVTT. Declared here, at the consumer.
type Tracks interface {
	// Available reports whether the tracks of a video can be listed on this
	// computer.
	Available() bool
	// Streams lists the audio and subtitle tracks of a video.
	Streams(ctx context.Context, id media.ID) (app.Streams, error)
	// Subtitle returns one embedded subtitle track as WebVTT.
	Subtitle(ctx context.Context, id media.ID, index int) ([]byte, error)
}

// tracksJSON is what a video holds besides its picture. Positions are those the
// converted stream and the subtitle route take.
type tracksJSON struct {
	Audio     []audioTrackJSON    `json:"audio"`
	Subtitles []embeddedTrackJSON `json:"subtitles"`
}

type audioTrackJSON struct {
	Index   int    `json:"index"`
	Label   string `json:"label"`
	Lang    string `json:"lang"`
	Default bool   `json:"default"`
}

type embeddedTrackJSON struct {
	Index int    `json:"index"`
	Label string `json:"label"`
	Lang  string `json:"lang"`
	// Supported is false for tracks made of pictures (Blu-ray, DVD), which cannot
	// be shown here.
	Supported bool `json:"supported"`
	Default   bool `json:"default"`
}

// handleTracks lists the audio and subtitle tracks inside a video. A video the
// computer cannot look into has none, and so does everything that is not a video:
// the page then offers no choice, as before.
func (a *api) handleTracks(w http.ResponseWriter, r *http.Request) {
	it, ok := a.lookup(w, r)
	if !ok {
		return
	}
	out := tracksJSON{Audio: []audioTrackJSON{}, Subtitles: []embeddedTrackJSON{}} // [] rather than null
	if a.tracks != nil {
		streams, err := a.tracks.Streams(r.Context(), it.ID)
		switch {
		case err == nil:
		case r.Context().Err() != nil:
			return
		default:
			// A file ffprobe cannot read plays or fails on its own; here it just has
			// no choices to offer.
			a.log.WarnContext(r.Context(), "could not list the tracks of a video", errAttr(err))
		}
		out = toTracksJSON(streams)
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	writeJSON(w, http.StatusOK, out)
}

// handleEmbeddedSubtitle serves one subtitle track stored inside a video as
// WebVTT. Like the sidecar route it takes an ID and a position, never a path.
func (a *api) handleEmbeddedSubtitle(w http.ResponseWriter, r *http.Request) {
	it, ok := a.lookup(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("track"))
	if a.tracks == nil || err != nil || n < 0 {
		writeError(w, http.StatusNotFound, codeNotFound, "no such subtitle")
		return
	}
	vtt, err := a.tracks.Subtitle(r.Context(), it.ID, n)
	switch {
	case err == nil:
	case errors.Is(err, media.ErrNotFound), errors.Is(err, app.ErrNoSubtitleTrack), errors.Is(err, app.ErrSubtitlesUnavailable):
		writeError(w, http.StatusNotFound, codeNotFound, "no such subtitle")
		return
	case r.Context().Err() != nil:
		return // the client left while waiting; nobody is listening
	default:
		a.log.ErrorContext(r.Context(), "read embedded subtitle failed", errAttr(err))
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	// A subtitle of a given file never changes (an edited file is a new ID and
	// size), and finding it is slow: let the browser keep it.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(vtt)
}

func toTracksJSON(s app.Streams) tracksJSON {
	out := tracksJSON{Audio: make([]audioTrackJSON, len(s.Audio)), Subtitles: make([]embeddedTrackJSON, len(s.Subtitles))}
	for i, t := range s.Audio {
		out.Audio[i] = audioTrackJSON{Index: t.Index, Label: audioLabel(t), Lang: t.Language, Default: t.Default}
	}
	for i, t := range s.Subtitles {
		out.Subtitles[i] = embeddedTrackJSON{Index: t.Index, Label: subtitleLabel(t), Lang: t.Language, Supported: t.Text, Default: t.Default}
	}
	return out
}

// audioLabel is how a track is named in a menu: "English · 5.1 · Dolby Digital".
func audioLabel(t app.AudioStream) string {
	parts := []string{languageOr(t.Language, t.Index)}
	if t.Title != "" && !strings.EqualFold(t.Title, parts[0]) {
		parts = append(parts, t.Title)
	}
	if l := channelLayout(t.Channels); l != "" {
		parts = append(parts, l)
	}
	if c := audioCodecName(t.Codec); c != "" {
		parts = append(parts, c)
	}
	return strings.Join(parts, " · ")
}

// subtitleLabel is how a track is named in a menu: "English (SDH)".
func subtitleLabel(t app.SubtitleStream) string {
	label := languageOr(t.Language, t.Index)
	if t.Title != "" && !strings.EqualFold(t.Title, label) {
		label += " (" + t.Title + ")"
	}
	return label
}

func languageOr(code string, index int) string {
	if name := languageName(code); name != "" {
		return name
	}
	return fmt.Sprintf("Track %d", index+1)
}

func channelLayout(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "Mono"
	case 2:
		return "Stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	default:
		return fmt.Sprintf("%d channels", n)
	}
}

var audioCodecNames = map[string]string{
	"aac": "AAC", "ac3": "Dolby Digital", "eac3": "Dolby Digital Plus", "dts": "DTS",
	"truehd": "TrueHD", "flac": "FLAC", "mp3": "MP3", "opus": "Opus", "vorbis": "Vorbis",
}

func audioCodecName(codec string) string {
	if n, ok := audioCodecNames[codec]; ok {
		return n
	}
	return strings.ToUpper(codec)
}

// languageNames covers the languages that turn up in home libraries. Files tag
// tracks with ISO 639-2 codes ("eng", "fre"/"fra"), sometimes ISO 639-1 ("en").
var languageNames = map[string]string{
	"eng": "English", "en": "English", "jpn": "Japanese", "ja": "Japanese", "kor": "Korean", "ko": "Korean",
	"chi": "Chinese", "zho": "Chinese", "zh": "Chinese", "spa": "Spanish", "es": "Spanish",
	"fre": "French", "fra": "French", "fr": "French", "ger": "German", "deu": "German", "de": "German",
	"ita": "Italian", "it": "Italian", "por": "Portuguese", "pt": "Portuguese", "rus": "Russian", "ru": "Russian",
	"tha": "Thai", "th": "Thai", "vie": "Vietnamese", "vi": "Vietnamese", "ind": "Indonesian", "id": "Indonesian",
	"msa": "Malay", "may": "Malay", "ms": "Malay", "hin": "Hindi", "hi": "Hindi", "ara": "Arabic", "ar": "Arabic",
	"tur": "Turkish", "tr": "Turkish", "pol": "Polish", "pl": "Polish", "dut": "Dutch", "nld": "Dutch", "nl": "Dutch",
	"swe": "Swedish", "sv": "Swedish", "nor": "Norwegian", "no": "Norwegian", "dan": "Danish", "da": "Danish",
	"fin": "Finnish", "fi": "Finnish", "ukr": "Ukrainian", "uk": "Ukrainian", "heb": "Hebrew", "he": "Hebrew",
	"gre": "Greek", "ell": "Greek", "el": "Greek", "cze": "Czech", "ces": "Czech", "cs": "Czech",
	"hun": "Hungarian", "hu": "Hungarian", "ron": "Romanian", "rum": "Romanian", "ro": "Romanian",
	"bur": "Burmese", "mya": "Burmese", "my": "Burmese", "tam": "Tamil", "ta": "Tamil", "tgl": "Filipino", "fil": "Filipino",
	"ben": "Bengali", "bn": "Bengali", "per": "Persian", "fas": "Persian", "fa": "Persian", "lat": "Latin",
}

// languageName names a language tag, or returns "" for one that is missing,
// undetermined ("und") or not in the table.
func languageName(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if i := strings.IndexAny(code, "-_"); i > 0 {
		code = code[:i] // "pt-BR" is Portuguese
	}
	return languageNames[code]
}
