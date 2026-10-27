package httpapi

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"eincast/internal/adapters/ffmpeg"
	"eincast/internal/adapters/thumbcache"
	"eincast/internal/app"
	"eincast/internal/app/tracks"
	"eincast/internal/app/transcode"
	"eincast/internal/domain/media"
)

// TestTracks_EndToEnd runs the whole path with the real parts: handlers, both
// services, ffprobe and ffmpeg. The video is HEVC with two audio languages and
// two embedded subtitle tracks, which is the case Phase 17 is for.
func TestTracks_EndToEnd(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	tc := ffmpeg.NewTranscoder(filepath.Join(t.TempDir(), "hls"), log)
	prober := ffmpeg.NewProber(log)
	if !tc.Available() || !prober.Available() {
		t.Skip("ffmpeg and ffprobe are not installed")
	}

	root := t.TempDir()
	srt := filepath.Join(root, "en.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:03,000\nHello there\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, _ := exec.LookPath("ffmpeg")
	if bin == "" {
		bin = "/opt/homebrew/bin/ffmpeg"
	}
	src := filepath.Join(root, "film.mkv")
	gen := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration=14",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=14",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-i", srt, "-i", srt,
		"-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s", "-map", "4:s",
		"-c:v", "libx265", "-pix_fmt", "yuv420p", "-c:a", "aac", "-c:s", "srt", "-t", "14",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=jpn",
		"-metadata:s:s:0", "language=eng", "-metadata:s:s:1", "language=fre", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("cannot generate a test video: %v: %s", err, out)
	}

	it := itemFor(t, root, "film.mkv", media.KindVideo)
	lib := &fakeLibrary{root: root, items: []media.Item{it}}
	conv := transcode.NewService(lib, tc, log, transcode.DefaultOptions)
	t.Cleanup(conv.StopAll)
	trk := tracks.NewService(lib, prober, ffmpeg.NewExtractor(log),
		thumbcache.New(filepath.Join(t.TempDir(), "subs"), log, thumbcache.WithFiles("subtitle", ".vtt")), log, tracks.DefaultOptions)

	a := New(lib, &fakeAccess{}, &fakeRemote{}, "1.2.3", discardLog(), WithTranscoding(conv), WithTracks(trk)).
		newAPI(app.ServeConfig{Name: "Den"})
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)
	id := string(it.ID)

	// What is inside.
	resp := getFrom(t, srv, "/api/library/"+id+"/tracks")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tracks status = %d", resp.StatusCode)
	}
	var got tracksJSON
	if err := json.Unmarshal(readBody(t, resp), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Audio) != 2 || got.Audio[0].Label != "English · Mono · AAC" || got.Audio[1].Lang != "jpn" {
		t.Errorf("audio = %+v", got.Audio)
	}
	if len(got.Subtitles) != 2 || got.Subtitles[1].Label != "French" || !got.Subtitles[1].Supported {
		t.Errorf("subtitles = %+v", got.Subtitles)
	}

	// An embedded subtitle, as WebVTT.
	sub := getFrom(t, srv, "/subtitle/"+id+"/embedded/1")
	body := string(readBody(t, sub))
	if sub.StatusCode != http.StatusOK || !strings.HasPrefix(body, "WEBVTT") || !strings.Contains(body, "Hello there") {
		t.Errorf("subtitle: status %d body %q", sub.StatusCode, body)
	}
	assertErrorShape(t, getFrom(t, srv, "/subtitle/"+id+"/embedded/2"), http.StatusNotFound, codeNotFound)

	// The second audio track is its own stream, with its own segment URLs.
	base := "/hls/" + id + "/audio/1/"
	pl := getFrom(t, srv, base+"index.m3u8")
	if pl.StatusCode != http.StatusOK {
		t.Fatalf("playlist of audio track 1: status %d: %s", pl.StatusCode, readBody(t, pl))
	}
	var seg string
	sc := bufio.NewScanner(strings.NewReader(string(readBody(t, pl))))
	for sc.Scan() {
		if line := sc.Text(); strings.HasSuffix(line, ".ts") {
			seg = line
			break
		}
	}
	if r := getFrom(t, srv, base+seg); r.StatusCode != http.StatusOK {
		t.Fatalf("segment %s of audio track 1: status %d", seg, r.StatusCode)
	}
	// The default track is a different conversion, not this one reused.
	if r := getFrom(t, srv, "/hls/"+id+"/index.m3u8"); r.StatusCode != http.StatusOK {
		t.Fatalf("playlist of the default track: status %d", r.StatusCode)
	}
	if n := conv.Running(); n != 2 {
		t.Errorf("%d conversions running, want one per audio track", n)
	}
	// A track the video does not have cannot be converted.
	assertErrorShape(t, getFrom(t, srv, "/hls/"+id+"/audio/5/index.m3u8"), http.StatusUnprocessableEntity, codeNotConvert)
}
