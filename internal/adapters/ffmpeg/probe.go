package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"eincast/internal/adapters/safepath"
	"eincast/internal/app"
)

// streamInfo is what ffprobe says about one stream of a file.
type streamInfo struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	PixFmt    string `json:"pix_fmt"`
	Channels  int    `json:"channels"`
	Tags      struct {
		Language string `json:"language"`
		Title    string `json:"title"`
	} `json:"tags"`
	Disposition struct {
		Default int `json:"default"`
	} `json:"disposition"`
}

// parseProbe reads `ffprobe -of json -show_entries stream=…` output.
func parseProbe(out []byte) ([]streamInfo, error) {
	var doc struct {
		Streams []streamInfo `json:"streams"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("read ffprobe output: %w", err)
	}
	return doc.Streams, nil
}

// probe asks ffprobe about the streams of a file.
func probe(ctx context.Context, ffprobePath, file string) ([]streamInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "error", "-protocol_whitelist", "file,pipe",
		"-show_entries", "stream=codec_type,codec_name,pix_fmt,channels:stream_tags=language,title:stream_disposition=default",
		"-of", "json", "file:"+file)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	return parseProbe(out)
}

// probeBinary finds ffprobe: the one installed beside ffmpeg, else on PATH, else
// "" (the video is then converted without looking first).
func probeBinary(ffmpegPath string) string {
	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	beside := filepath.Join(filepath.Dir(ffmpegPath), name)
	if info, err := os.Stat(beside); err == nil && !info.IsDir() {
		return beside
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p
	}
	return ""
}

// textSubtitleCodecs are the subtitle formats made of text, which ffmpeg turns
// into WebVTT. The others are pictures (Blu-ray PGS, DVD, DVB), which would need
// character recognition.
var textSubtitleCodecs = map[string]bool{
	"subrip": true, "srt": true, "ass": true, "ssa": true, "webvtt": true,
	"mov_text": true, "text": true, "subviewer": true, "microdvd": true, "sami": true,
}

// streamsOf sorts a probe into audio and subtitle tracks, numbered the way
// ffmpeg's "0:a:N" and "0:s:N" count them.
func streamsOf(all []streamInfo) app.Streams {
	var out app.Streams
	for _, s := range all {
		switch s.CodecType {
		case "audio":
			out.Audio = append(out.Audio, app.AudioStream{
				Index: len(out.Audio), Language: s.Tags.Language, Title: s.Tags.Title,
				Codec: s.CodecName, Channels: s.Channels, Default: s.Disposition.Default == 1,
			})
		case "subtitle":
			out.Subtitles = append(out.Subtitles, app.SubtitleStream{
				Index: len(out.Subtitles), Language: s.Tags.Language, Title: s.Tags.Title,
				Codec: s.CodecName, Text: textSubtitleCodecs[s.CodecName], Default: s.Disposition.Default == 1,
			})
		}
	}
	return out
}

// Prober lists the tracks inside a video with the installed ffprobe. It
// implements app.StreamProber.
type Prober struct {
	finder
}

var _ app.StreamProber = (*Prober)(nil)

// NewProber returns a Prober. Like the other ffmpeg users it looks for the
// program each time until it finds it.
func NewProber(log *slog.Logger, opts ...Option) *Prober {
	p := &Prober{}
	p.init(log, opts)
	return p
}

// Available reports whether ffprobe can be found beside the ffmpeg in use.
func (p *Prober) Available() bool {
	return p.probePath() != ""
}

func (p *Prober) probePath() string {
	bin, err := p.binary()
	if err != nil {
		return ""
	}
	return probeBinary(bin)
}

// Probe reads the tracks of a video. A file that is not media is an error.
func (p *Prober) Probe(ctx context.Context, req app.StreamProbe) (app.Streams, error) {
	path := p.probePath()
	if path == "" {
		return app.Streams{}, app.ErrSubtitlesUnavailable
	}
	// ffprobe opens whatever it is told, so the library guard applies here too.
	file, err := safepath.Inside(req.Root, req.Path)
	if err != nil {
		return app.Streams{}, fmt.Errorf("probe %q: %w", req.Path, err)
	}
	all, err := probe(ctx, path, file)
	if err != nil {
		return app.Streams{}, err
	}
	return streamsOf(all), nil
}
