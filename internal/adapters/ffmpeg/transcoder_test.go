package ffmpeg

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"eincast/internal/app"
)

func TestParseProbe(t *testing.T) {
	out := []byte(`{"programs":[],"streams":[
		{"index":0,"codec_name":"h264","codec_type":"video","pix_fmt":"yuv420p"},
		{"index":1,"codec_name":"ac3","codec_type":"audio","channels":6,"tags":{"language":"eng","title":"Surround"},"disposition":{"default":1}}]}`)
	got, err := parseProbe(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].CodecName != "h264" || got[0].PixFmt != "yuv420p" {
		t.Fatalf("parseProbe() = %+v", got)
	}
	a := got[1]
	if a.CodecType != "audio" || a.Channels != 6 || a.Tags.Language != "eng" || a.Tags.Title != "Surround" || a.Disposition.Default != 1 {
		t.Errorf("audio stream = %+v", a)
	}
	if _, err := parseProbe([]byte("not json")); err == nil {
		t.Error("parseProbe(garbage) = nil error")
	}
}

// stream builders for the tests below.
func vid(codec, pix string) streamInfo {
	return streamInfo{CodecType: "video", CodecName: codec, PixFmt: pix}
}
func aud(codec string, def bool) streamInfo {
	s := streamInfo{CodecType: "audio", CodecName: codec}
	if def {
		s.Disposition.Default = 1
	}
	return s
}

func TestMakePlan(t *testing.T) {
	tests := []struct {
		name    string
		streams []streamInfo
		track   int
		want    plan
	}{
		{"h264 and aac only need a new container", []streamInfo{vid("h264", "yuv420p"), aud("aac", false)}, app.DefaultAudioTrack, plan{copyVideo: true, copyAudio: true}},
		{"jpeg-range h264 is fine too", []streamInfo{vid("h264", "yuvj420p"), aud("aac", false)}, app.DefaultAudioTrack, plan{copyVideo: true, copyAudio: true}},
		{"hevc is converted", []streamInfo{vid("hevc", "yuv420p"), aud("aac", false)}, app.DefaultAudioTrack, plan{copyAudio: true}},
		{"10-bit h264 is converted (iOS cannot play it)", []streamInfo{vid("h264", "yuv420p10le"), aud("aac", false)}, app.DefaultAudioTrack, plan{copyAudio: true}},
		{"ac3 audio is converted", []streamInfo{vid("h264", "yuv420p"), aud("ac3", false)}, app.DefaultAudioTrack, plan{copyVideo: true}},
		{"no audio at all", []streamInfo{vid("h264", "yuv420p")}, app.DefaultAudioTrack, plan{copyVideo: true}},
		{"only the first video counts; the first audio is the default", []streamInfo{aud("aac", false), vid("hevc", "yuv420p"), vid("h264", "yuv420p"), aud("flac", false)}, app.DefaultAudioTrack, plan{copyAudio: true}},
		{"the track the file marks as default wins", []streamInfo{vid("h264", "yuv420p"), aud("ac3", false), aud("aac", true)}, app.DefaultAudioTrack, plan{copyVideo: true, copyAudio: true, audio: 1}},
		{"a chosen track is judged on its own codec", []streamInfo{vid("h264", "yuv420p"), aud("aac", false), aud("ac3", false)}, 1, plan{copyVideo: true, audio: 1}},
		{"a chosen track beats the default", []streamInfo{vid("h264", "yuv420p"), aud("ac3", true), aud("aac", false)}, 1, plan{copyVideo: true, copyAudio: true, audio: 1}},
		{"no probe result converts everything", nil, app.DefaultAudioTrack, plan{}},
		{"no probe result still honours a chosen track", nil, 2, plan{audio: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makePlan(tt.streams, tt.track)
			if err != nil {
				t.Fatal(err)
			}
			tt.want.encoder = encoderX264
			if got != tt.want {
				t.Fatalf("makePlan() = %+v, want %+v", got, tt.want)
			}
		})
	}

	t.Run("a track the file does not have is an error, not silence", func(t *testing.T) {
		streams := []streamInfo{vid("h264", "yuv420p"), aud("aac", false)}
		for _, track := range []int{1, 7, -2} {
			if _, err := makePlan(streams, track); err == nil {
				t.Errorf("makePlan(track %d) = nil error", track)
			}
		}
	})
}

func TestStreamsOf(t *testing.T) {
	sub := func(codec, lang string) streamInfo {
		s := streamInfo{CodecType: "subtitle", CodecName: codec}
		s.Tags.Language = lang
		return s
	}
	a := aud("aac", true)
	a.Tags.Language = "jpn"
	a.Channels = 2

	got := streamsOf([]streamInfo{vid("h264", "yuv420p"), a, sub("hdmv_pgs_subtitle", "eng"), aud("ac3", false), sub("subrip", "fre"), sub("ass", "")})

	if len(got.Audio) != 2 || got.Audio[0] != (app.AudioStream{Index: 0, Language: "jpn", Codec: "aac", Channels: 2, Default: true}) || got.Audio[1].Index != 1 {
		t.Errorf("Audio = %+v", got.Audio)
	}
	want := []struct {
		index int
		codec string
		text  bool
	}{{0, "hdmv_pgs_subtitle", false}, {1, "subrip", true}, {2, "ass", true}}
	if len(got.Subtitles) != len(want) {
		t.Fatalf("Subtitles = %+v", got.Subtitles)
	}
	for i, w := range want {
		if s := got.Subtitles[i]; s.Index != w.index || s.Codec != w.codec || s.Text != w.text {
			t.Errorf("Subtitles[%d] = %+v, want %+v", i, s, w)
		}
	}
	if empty := streamsOf(nil); empty.Audio != nil || empty.Subtitles != nil {
		t.Errorf("streamsOf(nil) = %+v", empty)
	}
}

func TestHLSArgs(t *testing.T) {
	has := func(args []string, pair ...string) bool {
		for i := 0; i+len(pair) <= len(args); i++ {
			if slices.Equal(args[i:i+len(pair)], pair) {
				return true
			}
		}
		return false
	}

	t.Run("remux copies both streams and does not encode", func(t *testing.T) {
		a := hlsArgs("/lib/a.mkv", "/out", plan{copyVideo: true, copyAudio: true})
		if !has(a, "-c:v", "copy") || !has(a, "-c:a", "copy") || slices.Contains(a, "libx264") {
			t.Errorf("args = %v", a)
		}
	})

	t.Run("conversion encodes H.264 + AAC with an even keyframe rhythm", func(t *testing.T) {
		a := hlsArgs("/lib/a.mkv", "/out", plan{encoder: encoderX264})
		for _, pair := range [][]string{
			{"-c:v", "libx264"}, {"-pix_fmt", "yuv420p"}, {"-c:a", "aac"}, {"-ac", "2"}, {"-sc_threshold", "0"},
			{"-force_key_frames", "expr:gte(t,n_forced*6)"},
		} {
			if !has(a, pair...) {
				t.Errorf("args lack %v: %v", pair, a)
			}
		}
	})

	t.Run("reads the input as a file and keeps a missing audio track optional", func(t *testing.T) {
		a := hlsArgs("/lib/concat:x.mkv", "/out", plan{})
		if !has(a, "-i", "file:/lib/concat:x.mkv") || !has(a, "-protocol_whitelist", "file,pipe") || !has(a, "-map", "0:a:0?") {
			t.Errorf("args = %v", a)
		}
	})

	t.Run("maps the chosen audio stream", func(t *testing.T) {
		a := hlsArgs("/lib/a.mkv", "/out", plan{audio: 2, encoder: encoderX264})
		if !has(a, "-map", "0:a:2?") {
			t.Errorf("args = %v", a)
		}
	})

	t.Run("a chip encoder replaces libx264 and keeps the keyframe rhythm", func(t *testing.T) {
		for _, enc := range []encoder{encoderVideoToolbox, encoderNVENC, encoderQSV} {
			a := hlsArgs("/lib/a.mkv", "/out", plan{encoder: enc})
			if !has(a, "-c:v", string(enc)) || slices.Contains(a, "libx264") || !has(a, "-force_key_frames", "expr:gte(t,n_forced*6)") {
				t.Errorf("%s: args = %v", enc, a)
			}
		}
	})

	t.Run("a copied picture names no encoder", func(t *testing.T) {
		a := hlsArgs("/lib/a.mkv", "/out", plan{copyVideo: true, encoder: encoderVideoToolbox})
		if slices.Contains(a, string(encoderVideoToolbox)) {
			t.Errorf("args = %v", a)
		}
	})

	t.Run("a percent sign in the folder cannot become a pattern", func(t *testing.T) {
		a := hlsArgs("/lib/a.mkv", "/out/100%/s-1", plan{})
		if !has(a, "-hls_segment_filename", "/out/100%%/s-1/seg%05d.ts") {
			t.Errorf("args = %v", a)
		}
	})

	t.Run("writes into the folder it is given", func(t *testing.T) {
		a := hlsArgs("/lib/a.mkv", "/out/s-1", plan{})
		if a[len(a)-1] != "/out/s-1/index.m3u8" {
			t.Errorf("last arg = %q", a[len(a)-1])
		}
	})
}

func TestOutputNames(t *testing.T) {
	for name, want := range map[string]bool{
		"index.m3u8": true, "seg00000.ts": true, "seg12345.ts": true,
		"../index.m3u8": false, "seg0.ts": false, "seg00000.ts/": false, "index.m3u8.tmp": false,
		"seg00000.ts.tmp": false, "a/seg00000.ts": false, "": false, `..\seg00000.ts`: false,
	} {
		if got := outputName.MatchString(name); got != want {
			t.Errorf("outputName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestTranscoder_WithoutFFmpeg(t *testing.T) {
	tc := NewTranscoder(t.TempDir(), slog.New(slog.DiscardHandler))
	tc.find = func() (string, error) { return "", errors.New("not found") }
	if tc.Available() {
		t.Error("Available() = true")
	}
	if _, err := tc.Start(context.Background(), app.TranscodeRequest{Root: "/lib", Path: "/lib/a.mkv"}); !errors.Is(err, app.ErrTranscodingUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestNewTranscoder_ClearsWhatAnEarlierRunLeft(t *testing.T) {
	base := t.TempDir()
	stale := filepath.Join(base, "s-old")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "seg00000.ts"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	NewTranscoder(base, slog.New(slog.DiscardHandler))
	if entries, _ := os.ReadDir(base); len(entries) != 0 {
		t.Fatalf("base folder still holds %d entries", len(entries))
	}
}

// The tests below run the real programs and are skipped where they are missing.

func realTranscoder(t *testing.T) (*Transcoder, string) {
	t.Helper()
	base := t.TempDir()
	tc := NewTranscoder(filepath.Join(base, "hls"), slog.New(slog.DiscardHandler))
	if !tc.Available() {
		t.Skip("ffmpeg is not installed")
	}
	return tc, base
}

func makeVideo(t *testing.T, tc *Transcoder, name string, args ...string) string {
	t.Helper()
	bin, _ := tc.binary()
	path := filepath.Join(t.TempDir(), name)
	full := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
	full = append(full, path)
	if out, err := exec.Command(bin, full...).CombinedOutput(); err != nil {
		t.Skipf("cannot generate %s with this ffmpeg: %v: %s", name, err, out)
	}
	return path
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(60 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func readStreamFile(t *testing.T, s app.HLSStream, name string) string {
	t.Helper()
	f, err := s.Open(name)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func start(t *testing.T, tc *Transcoder, path string) app.HLSStream {
	t.Helper()
	s, err := tc.Start(context.Background(), app.TranscodeRequest{Root: filepath.Dir(path), Path: path, AudioTrack: app.DefaultAudioTrack})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

func TestTranscoder_RemuxesH264ToHLS(t *testing.T) {
	tc, _ := realTranscoder(t)
	path := makeVideo(t, tc, "src.mkv",
		"-f", "lavfi", "-i", "testsrc=size=640x360:rate=25:duration=15",
		"-f", "lavfi", "-i", "sine=duration=15",
		"-c:v", "libx264", "-g", "50", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest")

	s := start(t, tc, path)
	waitFor(t, s.Ready(), "the stream to be ready")
	waitFor(t, s.Done(), "the conversion to finish")
	if err := s.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}

	playlist := readStreamFile(t, s, "index.m3u8")
	if !strings.HasPrefix(playlist, "#EXTM3U") || !strings.Contains(playlist, "seg00000.ts") || !strings.Contains(playlist, "#EXT-X-ENDLIST") {
		t.Errorf("playlist =\n%s", playlist)
	}
	if seg := readStreamFile(t, s, "seg00000.ts"); len(seg) < 188 || seg[0] != 0x47 {
		t.Errorf("segment does not look like MPEG-TS (len %d)", len(seg))
	}
}

func TestTranscoder_ConvertsHEVCToH264(t *testing.T) {
	tc, _ := realTranscoder(t)
	path := makeVideo(t, tc, "hevc.mkv",
		"-f", "lavfi", "-i", "testsrc=size=640x360:rate=25:duration=14",
		"-f", "lavfi", "-i", "sine=duration=14",
		"-c:v", "libx265", "-pix_fmt", "yuv420p", "-c:a", "libmp3lame", "-shortest")

	s := start(t, tc, path)
	waitFor(t, s.Ready(), "the stream to be ready")
	waitFor(t, s.Done(), "the conversion to finish")
	if err := s.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}

	// What came out must be H.264 + AAC, which is what makes it playable on an iPhone.
	bin, _ := tc.binary()
	probe := probeBinary(bin)
	if probe == "" {
		t.Skip("ffprobe is not installed")
	}
	seg := filepath.Join(filepath.Dir(path), "check.ts")
	data := readStreamFile(t, s, "seg00000.ts")
	if err := os.WriteFile(seg, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	streams, err := func() ([]streamInfo, error) { return probeFile(probe, seg) }()
	if err != nil {
		t.Fatal(err)
	}
	if p, err := makePlan(streams, app.DefaultAudioTrack); err != nil || !p.copyVideo || !p.copyAudio {
		t.Errorf("output streams = %+v (err %v), want 8-bit H.264 and AAC", streams, err)
	}
}

func probeFile(probePath, file string) ([]streamInfo, error) {
	return probe(context.Background(), probePath, file)
}

func TestTranscoder_ReadyBeforeTheEndOfALongConversion(t *testing.T) {
	tc, _ := realTranscoder(t)
	// Long enough that a full software conversion takes seconds; ready must come
	// after two segments (12 s of video), not at the end.
	path := makeVideo(t, tc, "long.mkv",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30:duration=300",
		"-c:v", "mpeg4", "-q:v", "8")

	s := start(t, tc, path)
	waitFor(t, s.Ready(), "the stream to be ready")
	select {
	case <-s.Done():
		t.Fatal("the whole video was converted before Ready: no streaming benefit")
	default:
	}
	if !strings.Contains(readStreamFile(t, s, "index.m3u8"), "seg00001.ts") {
		t.Error("playlist does not list the second segment yet")
	}
}

func TestTranscoder_StopKillsFFmpegAndDeletesTheFiles(t *testing.T) {
	tc, base := realTranscoder(t)
	path := makeVideo(t, tc, "long.mkv",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30:duration=300",
		"-c:v", "mpeg4", "-q:v", "8")
	s := start(t, tc, path)
	waitFor(t, s.Ready(), "the stream to be ready")

	s.Stop()
	select {
	case <-s.Done():
	default:
		t.Fatal("Stop() returned while ffmpeg was still running")
	}
	if err := s.Err(); err != nil {
		t.Errorf("Err() = %v, want nil for a requested stop", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(base, "hls")); len(entries) != 0 {
		t.Errorf("files left behind: %v", entries)
	}
	if _, err := s.Open("index.m3u8"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open after Stop: error = %v, want fs.ErrNotExist", err)
	}
	s.Stop() // a second call is harmless
}

func TestTranscoder_AFileThatIsNotVideoFailsWithFFmpegsReason(t *testing.T) {
	tc, _ := realTranscoder(t)
	path := filepath.Join(t.TempDir(), "broken.mkv")
	if err := os.WriteFile(path, []byte("this is not a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := start(t, tc, path)
	waitFor(t, s.Done(), "the conversion to fail")
	if err := s.Err(); err == nil || !strings.Contains(err.Error(), "ffmpeg:") {
		t.Fatalf("Err() = %v, want ffmpeg's reason", err)
	}
	select {
	case <-s.Ready():
		t.Error("Ready closed for a conversion that failed")
	default:
	}
}

func TestTranscoder_OnlyKnownFilesCanBeOpened(t *testing.T) {
	tc, _ := realTranscoder(t)
	path := makeVideo(t, tc, "src.mkv", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration=2", "-c:v", "libx264", "-pix_fmt", "yuv420p")
	s := start(t, tc, path)
	waitFor(t, s.Done(), "the conversion to finish")

	for _, name := range []string{"../src.mkv", "seg99999.ts", "other.txt", "index.m3u8/../../x"} {
		if _, err := s.Open(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open(%q) error = %v, want fs.ErrNotExist", name, err)
		}
	}
}

func TestTranscoder_RefusesAFileOutsideTheLibrary(t *testing.T) {
	tc, _ := realTranscoder(t)
	outside := filepath.Join(t.TempDir(), "x.mkv")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := tc.Start(context.Background(), app.TranscodeRequest{Root: t.TempDir(), Path: outside})
	if err == nil || errors.Is(err, app.ErrTranscodingUnavailable) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}
