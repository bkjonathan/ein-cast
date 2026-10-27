package ffmpeg

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"eincast/internal/app"
)

const encodersOutput = `Encoders:
 V..... = Video
 ------
 V....D libx264              libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 V....D h264_videotoolbox    VideoToolbox H.264 Encoder (codec h264)
 V....D h264_nvenc           NVIDIA NVENC H.264 encoder (codec h264)
 A....D aac                  AAC (Advanced Audio Coding)
`

func TestListedEncoders(t *testing.T) {
	tests := []struct {
		name       string
		out        string
		candidates []encoder
		want       []encoder
	}{
		{"finds the ones the build has, in the order asked", encodersOutput, []encoder{encoderNVENC, encoderQSV, encoderVideoToolbox}, []encoder{encoderNVENC, encoderVideoToolbox}},
		{"a name inside a description is not an encoder", " V....D libx264  uses h264_qsv ideas\n", []encoder{encoderQSV}, nil},
		{"none", "", []encoder{encoderNVENC}, nil},
		{"no candidates", encodersOutput, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listedEncoders(tt.out, tt.candidates); !slices.Equal(got, tt.want) {
				t.Fatalf("listedEncoders() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHardwareCandidates(t *testing.T) {
	if got := hardwareCandidates("darwin"); !slices.Equal(got, []encoder{encoderVideoToolbox}) {
		t.Errorf("darwin = %v", got)
	}
	if got := hardwareCandidates("windows"); !slices.Equal(got, []encoder{encoderNVENC, encoderQSV}) {
		t.Errorf("windows = %v", got)
	}
	if got := hardwareCandidates("linux"); got != nil {
		t.Errorf("linux = %v", got)
	}
}

func TestEncoderChain_SoftwareIsAlwaysTheLastResort(t *testing.T) {
	hw := []encoder{encoderNVENC, encoderQSV}
	if got := encoderChain(hw, true); !slices.Equal(got, []encoder{encoderNVENC, encoderQSV, encoderX264}) {
		t.Errorf("on = %v", got)
	}
	if got := encoderChain(hw, false); !slices.Equal(got, []encoder{encoderX264}) {
		t.Errorf("off = %v", got)
	}
	if got := encoderChain(nil, true); !slices.Equal(got, []encoder{encoderX264}) {
		t.Errorf("none found = %v", got)
	}
}

func TestSubtitleArgs(t *testing.T) {
	a := subtitleArgs("/lib/a.mkv", 2)
	joined := strings.Join(a, " ")
	for _, want := range []string{"-i file:/lib/a.mkv", "-protocol_whitelist file,pipe", "-map 0:s:2", "-c:s webvtt", "-f webvtt pipe:1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args lack %q: %v", want, a)
		}
	}
}

func TestTranscoder_RefusesWhenTheDiskIsTooFull(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "film.mkv")
	if err := os.WriteFile(file, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	tc := NewTranscoder(filepath.Join(t.TempDir(), "hls"), slog.New(slog.DiscardHandler))
	tc.find = func() (string, error) { return "/bin/true", nil } // never reached when refused

	free := uint64(diskHeadroom) // room for the headroom but not for the output
	tc.freeBytes = func(string) (uint64, error) { return free, nil }
	_, err := tc.Start(context.Background(), app.TranscodeRequest{Root: root, Path: file})
	if !errors.Is(err, app.ErrNotEnoughDiskSpace) {
		t.Fatalf("Start() error = %v, want ErrNotEnoughDiskSpace", err)
	}
	entries, _ := os.ReadDir(tc.baseDir)
	if len(entries) != 0 {
		t.Errorf("a refused conversion left %d folders behind", len(entries))
	}
}

func TestCheckDiskSpace(t *testing.T) {
	root := t.TempDir()
	small := filepath.Join(root, "small.mkv")
	if err := os.WriteFile(small, make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	tc := NewTranscoder(filepath.Join(t.TempDir(), "hls"), slog.New(slog.DiscardHandler))

	tests := []struct {
		name string
		free uint64
		err  error // from the free-space call
		want error
	}{
		{"plenty", 100 << 30, nil, nil},
		{"just enough", diskHeadroom + 1000, nil, nil},
		{"one byte short", diskHeadroom + 999, nil, app.ErrNotEnoughDiskSpace},
		{"full", 0, nil, app.ErrNotEnoughDiskSpace},
		{"not being able to ask is not a reason to refuse", 0, errors.New("no statfs"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc.freeBytes = func(string) (uint64, error) { return tt.free, tt.err }
			if err := tc.checkDiskSpace(small); !errors.Is(err, tt.want) {
				t.Fatalf("checkDiskSpace() = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("a huge source is estimated at the cap, not its own size", func(t *testing.T) {
		// A sparse file: 60 GB on paper, nothing on disk.
		huge := filepath.Join(root, "huge.mkv")
		f, err := os.Create(huge)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(60 << 30); err != nil {
			t.Skip("cannot make a sparse file here")
		}
		f.Close()
		tc.freeBytes = func(string) (uint64, error) { return diskEstimateCap + diskHeadroom, nil }
		if err := tc.checkDiskSpace(huge); err != nil {
			t.Fatalf("checkDiskSpace() = %v", err)
		}
	})
}

func TestFreeBytes_ReportsTheRealDisk(t *testing.T) {
	free, err := freeBytes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if free == 0 {
		t.Error("freeBytes() = 0 on a disk that holds a test")
	}
	if _, err := freeBytes(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("freeBytes(missing folder) = nil error")
	}
}

// fakeFFmpeg writes a stand-in for ffmpeg that fails whenever it is asked for
// the encoder named in failOn ("all" for every one), and otherwise writes a playlist where ffmpeg
// would. It appends every encoder it is asked for to a log file.
func fakeFFmpeg(t *testing.T, advertises, failOn string) (bin, calls string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls.txt")
	bin = filepath.Join(dir, "ffmpeg")
	script := `#!/bin/sh
case "$*" in
  *-encoders*) echo " V....D ` + advertises + `   a chip encoder"; exit 0 ;;
esac
for a in "$@"; do last="$a"; done
enc=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-c:v" ]; then enc="$a"; fi
  prev="$a"
done
echo "$enc" >> "` + calls + `"
if [ "` + failOn + `" = "all" ] || [ "$enc" = "` + failOn + `" ]; then
  echo "Cannot initialise the encoder" >&2
  exit 1
fi
printf '#EXTM3U\n#EXT-X-ENDLIST\n' > "$last"
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, calls
}

func fakeTranscoder(t *testing.T, bin string, hardware bool) (*Transcoder, string) {
	t.Helper()
	tc := NewTranscoder(filepath.Join(t.TempDir(), "hls"), slog.New(slog.DiscardHandler),
		WithHardwareEncoding(func() bool { return hardware }))
	tc.find = func() (string, error) { return bin, nil }
	tc.goos = "darwin"
	root := t.TempDir()
	file := filepath.Join(root, "film.mkv")
	if err := os.WriteFile(file, []byte("not really a film"), 0o644); err != nil {
		t.Fatal(err)
	}
	return tc, file
}

func readCalls(t *testing.T, calls string) []string {
	t.Helper()
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(b))
}

func TestTranscoder_AFailingChipFallsBackToTheProcessor(t *testing.T) {
	bin, calls := fakeFFmpeg(t, "h264_videotoolbox", "h264_videotoolbox")
	tc, file := fakeTranscoder(t, bin, true)

	s := start(t, tc, file)
	waitFor(t, s.Done(), "the conversion to end")
	if err := s.Err(); err != nil {
		t.Fatalf("Err() = %v, want the software encoder to have taken over", err)
	}
	if got := readCalls(t, calls); !slices.Equal(got, []string{"h264_videotoolbox", "libx264"}) {
		t.Errorf("encoders tried = %v", got)
	}
	if !strings.HasPrefix(readStreamFile(t, s, "index.m3u8"), "#EXTM3U") {
		t.Error("no playlist after the fallback")
	}
}

func TestTranscoder_AWorkingChipIsUsedAlone(t *testing.T) {
	bin, calls := fakeFFmpeg(t, "h264_videotoolbox", "")
	tc, file := fakeTranscoder(t, bin, true)

	s := start(t, tc, file)
	waitFor(t, s.Done(), "the conversion to end")
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if got := readCalls(t, calls); !slices.Equal(got, []string{"h264_videotoolbox"}) {
		t.Errorf("encoders tried = %v", got)
	}
}

func TestTranscoder_TheSettingTurnsTheChipOff(t *testing.T) {
	bin, calls := fakeFFmpeg(t, "h264_videotoolbox", "")
	tc, file := fakeTranscoder(t, bin, false)

	s := start(t, tc, file)
	waitFor(t, s.Done(), "the conversion to end")
	if got := readCalls(t, calls); !slices.Equal(got, []string{"libx264"}) {
		t.Errorf("encoders tried = %v, want only the processor", got)
	}
}

func TestTranscoder_WhenEverythingFailsTheLastReasonIsReported(t *testing.T) {
	bin, _ := fakeFFmpeg(t, "h264_videotoolbox", "all")
	tc, file := fakeTranscoder(t, bin, true)

	s := start(t, tc, file)
	waitFor(t, s.Done(), "the conversion to end")
	if err := s.Err(); err == nil || !strings.Contains(err.Error(), "Cannot initialise the encoder") {
		t.Fatalf("Err() = %v", err)
	}
}

func TestTranscoder_ChipEncodersAreLookedUpOncePerProgram(t *testing.T) {
	bin, _ := fakeFFmpeg(t, "h264_videotoolbox", "")
	tc, _ := fakeTranscoder(t, bin, true)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tc.encoderChain(context.Background(), bin)
		}()
	}
	wg.Wait()
	if got := tc.encoderChain(context.Background(), bin); !slices.Equal(got, []encoder{encoderVideoToolbox, encoderX264}) {
		t.Errorf("chain = %v", got)
	}
}

// The tests below run the real programs and are skipped where they are missing.

// meanVolume measures how loud the first audio stream of a file is, in dB.
func meanVolume(t *testing.T, tc *Transcoder, file string) float64 {
	t.Helper()
	bin, _ := tc.binary()
	out, err := exec.Command(bin, "-hide_banner", "-nostdin", "-i", file, "-af", "volumedetect", "-vn", "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("measure volume: %v: %s", err, out)
	}
	m := regexp.MustCompile(`mean_volume: (-?[0-9.]+|-inf) dB`).FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("no volume in:\n%s", out)
	}
	if m[1] == "-inf" {
		return -200
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v
}

func concatSegments(t *testing.T, s app.HLSStream, out string) {
	t.Helper()
	data := readStreamFile(t, s, "seg00000.ts")
	if err := os.WriteFile(out, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTranscoder_ConvertsTheChosenAudioTrack(t *testing.T) {
	tc, _ := realTranscoder(t)
	// Track 0 is silent, track 1 is a loud tone: which one came out tells which
	// was mapped.
	path := makeVideo(t, tc, "two.mkv",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration=8",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=8",
		"-map", "0:v", "-map", "1:a", "-map", "2:a",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", "8",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=jpn")

	for track, loud := range map[int]bool{0: false, 1: true, app.DefaultAudioTrack: false} {
		s, err := tc.Start(context.Background(), app.TranscodeRequest{Root: filepath.Dir(path), Path: path, AudioTrack: track})
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, s.Done(), "the conversion to finish")
		if err := s.Err(); err != nil {
			t.Fatalf("track %d: Err() = %v", track, err)
		}
		seg := filepath.Join(t.TempDir(), "check.ts")
		concatSegments(t, s, seg)
		got := meanVolume(t, tc, seg)
		s.Stop()
		if loud && got < -40 {
			t.Errorf("track %d: mean volume %.1f dB, want the loud track", track, got)
		}
		if !loud && got > -60 {
			t.Errorf("track %d: mean volume %.1f dB, want the silent track", track, got)
		}
	}

	s, err := tc.Start(context.Background(), app.TranscodeRequest{Root: filepath.Dir(path), Path: path, AudioTrack: 5})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	waitFor(t, s.Done(), "the conversion to end")
	if err := s.Err(); err == nil || !strings.Contains(err.Error(), "no audio track 6") {
		t.Errorf("a missing track: Err() = %v", err)
	}
}

func TestTranscoder_ProbesAndExtractsEmbeddedSubtitles(t *testing.T) {
	tc, _ := realTranscoder(t)
	dir := t.TempDir()
	srt := filepath.Join(dir, "en.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:03,000\nHello there\n\n2\n00:00:04,000 --> 00:00:05,500\nSecond line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := makeVideo(t, tc, "subs.mkv",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=10:duration=6",
		"-f", "lavfi", "-i", "sine=duration=6",
		"-i", srt,
		"-map", "0:v", "-map", "1:a", "-map", "2:s",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-c:s", "srt",
		"-metadata:s:s:0", "language=eng", "-metadata:s:s:0", "title=English")

	root := filepath.Dir(path)
	prober := NewProber(slog.New(slog.DiscardHandler))
	if !prober.Available() {
		t.Skip("ffprobe is not installed")
	}
	got, err := prober.Probe(context.Background(), app.StreamProbe{Root: root, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Audio) != 1 || len(got.Subtitles) != 1 {
		t.Fatalf("Probe() = %+v", got)
	}
	if s := got.Subtitles[0]; s.Language != "eng" || s.Title != "English" || !s.Text || s.Index != 0 {
		t.Errorf("subtitle = %+v", s)
	}

	ex := NewExtractor(slog.New(slog.DiscardHandler))
	vtt, err := ex.Extract(context.Background(), app.SubtitleExtraction{Root: root, Path: path, Index: 0})
	if err != nil {
		t.Fatal(err)
	}
	text := string(vtt)
	if !strings.HasPrefix(text, "WEBVTT") || !strings.Contains(text, "00:01.") || !strings.Contains(text, " --> 00:03.") || !strings.Contains(text, "Hello there") {
		t.Errorf("WebVTT =\n%s", text)
	}

	if _, err := ex.Extract(context.Background(), app.SubtitleExtraction{Root: root, Path: path, Index: 3}); !errors.Is(err, app.ErrNoSubtitleTrack) {
		t.Errorf("a track that is not there: error = %v", err)
	}
	if _, err := ex.Extract(context.Background(), app.SubtitleExtraction{Root: root, Path: "/etc/passwd", Index: 0}); err == nil {
		t.Error("a file outside the library was opened")
	}
}

func TestTranscoder_HardwareEncodeProducesH264(t *testing.T) {
	tc, _ := realTranscoder(t)
	bin, _ := tc.binary()
	if len(detectEncoders(context.Background(), bin, hardwareCandidates(runtime.GOOS))) == 0 {
		t.Skip("this ffmpeg has no chip encoder for this computer")
	}
	var logs strings.Builder
	hw := NewTranscoder(filepath.Join(t.TempDir(), "hls"),
		slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})),
		WithHardwareEncoding(func() bool { return true }))
	path := makeVideo(t, tc, "hevc.mkv",
		"-f", "lavfi", "-i", "testsrc=size=640x360:rate=25:duration=14",
		"-f", "lavfi", "-i", "sine=duration=14",
		"-c:v", "libx265", "-pix_fmt", "yuv420p", "-c:a", "libmp3lame", "-shortest")

	s := start(t, hw, path)
	waitFor(t, s.Ready(), "the stream to be ready")
	waitFor(t, s.Done(), "the conversion to finish")
	if err := s.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if strings.Contains(logs.String(), "trying another") {
		t.Skipf("the chip was refused on this computer, so the processor did the work:\n%s", logs.String())
	}

	probePath := probeBinary(bin)
	if probePath == "" {
		t.Skip("ffprobe is not installed")
	}
	seg := filepath.Join(t.TempDir(), "check.ts")
	concatSegments(t, s, seg)
	streams, err := probe(context.Background(), probePath, seg)
	if err != nil {
		t.Fatal(err)
	}
	if v := streams[0]; v.CodecName != "h264" || v.PixFmt != "yuv420p" {
		t.Errorf("video = %+v, want 8-bit H.264", v)
	}
}
