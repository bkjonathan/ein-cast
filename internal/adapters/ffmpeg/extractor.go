package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"eincast/internal/adapters/safepath"
	"eincast/internal/app"
)

// maxSubtitleOutput bounds one extracted track. Real subtitles are tens to
// hundreds of kilobytes; this stops a mislabelled stream from filling memory.
const maxSubtitleOutput = 4 << 20

// extractTimeout caps one extraction. Finding a track means reading the whole
// file, which on a network drive can be slow, but not endless.
const extractTimeout = 3 * time.Minute

// Extractor reads subtitle tracks out of videos with the installed ffmpeg. It
// implements app.SubtitleExtractor.
type Extractor struct {
	finder
}

var _ app.SubtitleExtractor = (*Extractor)(nil)

// NewExtractor returns an Extractor.
func NewExtractor(log *slog.Logger, opts ...Option) *Extractor {
	e := &Extractor{}
	e.init(log.With(slog.String("source", "subtitles")), opts)
	return e
}

// subtitleArgs builds the ffmpeg command line that writes one text subtitle
// track of a file as WebVTT to standard output. The input is "file:<path>" and
// only the file and pipe protocols are allowed, as everywhere else.
func subtitleArgs(file string, index int) []string {
	return []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-protocol_whitelist", "file,pipe",
		"-i", "file:" + file,
		"-map", fmt.Sprintf("0:s:%d", index),
		"-c:s", "webvtt", "-f", "webvtt", "pipe:1",
	}
}

// Extract returns the track as WebVTT.
func (e *Extractor) Extract(ctx context.Context, req app.SubtitleExtraction) ([]byte, error) {
	bin, err := e.binary()
	if err != nil {
		return nil, app.ErrSubtitlesUnavailable
	}
	file, err := safepath.Inside(req.Root, req.Path)
	if err != nil {
		return nil, fmt.Errorf("read subtitles of %q: %w", req.Path, err)
	}
	if req.Index < 0 {
		return nil, app.ErrNoSubtitleTrack
	}

	ctx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, subtitleArgs(file, req.Index)...)
	hideWindow(cmd)
	var stdout, stderr bytes.Buffer
	out := &limitedWriter{w: &stdout, left: maxSubtitleOutput + 1}
	cmd.Stdout = out
	cmd.Stderr = &limitedWriter{w: &stderr, left: 4 << 10}

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// ffmpeg ends with a generic line, so the reason is looked for in all of it.
			all, reason := stderr.String(), lastLine(stderr.String())
			// No such stream, or one ffmpeg cannot write as text (a picture).
			if strings.Contains(all, "matches no streams") || strings.Contains(all, "only possible from text") {
				return nil, fmt.Errorf("%w: ffmpeg: %s", app.ErrNoSubtitleTrack, reason)
			}
			return nil, fmt.Errorf("ffmpeg: %s", reason)
		}
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	if stdout.Len() > maxSubtitleOutput {
		return nil, fmt.Errorf("%w: the track is too large", app.ErrNoSubtitleTrack)
	}
	if !bytes.HasPrefix(stdout.Bytes(), []byte("WEBVTT")) {
		return nil, fmt.Errorf("%w: ffmpeg produced no subtitles", app.ErrNoSubtitleTrack)
	}
	return stdout.Bytes(), nil
}
