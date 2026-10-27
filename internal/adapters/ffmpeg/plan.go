package ffmpeg

import (
	"fmt"
	"strings"

	"eincast/internal/app"
)

// plan says how each stream gets into the HLS output: copied as it is (fast and
// lossless) or converted.
type plan struct {
	copyVideo bool
	copyAudio bool
	// audio is the position of the audio stream to use among the file's audio
	// streams.
	audio int
	// encoder makes the picture when copyVideo is false.
	encoder encoder
}

// makePlan decides from the probe. What Safari, iOS and TVs all play inside
// HLS is 8-bit H.264 with AAC audio; a file that already has that only needs a
// new container, which takes a moment, while anything else (HEVC, 10-bit, AC-3,
// FLAC, …) is converted. With no probe result, everything is converted: slower,
// but never wrong.
//
// audioTrack picks the audio stream; app.DefaultAudioTrack picks the one the
// file marks as default, else the first. A position the file does not have is
// an error here, because ffmpeg would otherwise quietly leave the sound out; a
// file with no sound at all has nothing to choose, so any position is fine.
func makePlan(streams []streamInfo, audioTrack int) (plan, error) {
	p := plan{encoder: encoderX264}
	var audio []streamInfo
	seenVideo := false
	for _, s := range streams {
		switch {
		case s.CodecType == "video" && !seenVideo:
			seenVideo = true
			p.copyVideo = s.CodecName == "h264" && (s.PixFmt == "yuv420p" || s.PixFmt == "yuvj420p")
		case s.CodecType == "audio":
			audio = append(audio, s)
		}
	}

	switch {
	case audioTrack == app.DefaultAudioTrack:
		for i, s := range audio {
			if s.Disposition.Default == 1 {
				audioTrack = i
				break
			}
		}
		if audioTrack == app.DefaultAudioTrack {
			audioTrack = 0
		}
	case len(audio) > 0 && audioTrack >= len(audio):
		return plan{}, fmt.Errorf("the video has no audio track %d", audioTrack+1)
	case audioTrack < 0:
		return plan{}, fmt.Errorf("no audio track %d", audioTrack)
	}
	p.audio = audioTrack
	if audioTrack < len(audio) {
		p.copyAudio = audio[audioTrack].CodecName == "aac"
	}
	return p, nil
}

// segmentSeconds is the target length of one HLS segment. Short enough to start
// quickly and seek in, long enough to keep the playlist small.
const segmentSeconds = 6

// hlsArgs builds the ffmpeg command line that writes index.m3u8 and its
// segments into dir. As with thumbnails, the input is "file:<path>" and only the
// file and pipe protocols are allowed.
func hlsArgs(file, dir string, p plan) []string {
	a := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-protocol_whitelist", "file,pipe",
		"-i", "file:" + file,
		// The first video stream, and the chosen audio stream if there is one.
		"-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d?", p.audio),
		"-sn",
	}
	if p.copyVideo {
		a = append(a, "-c:v", "copy")
	} else {
		a = append(a, p.encoder.args()...)
		a = append(a,
			// Never bigger than 1080p wide: more costs time and bandwidth for no
			// visible gain on a phone or a TV across a room.
			"-vf", "scale='min(1920,iw)':-2",
			// A keyframe at every segment boundary, so segments are an even length
			// and a player can start at any of them.
			"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", segmentSeconds),
			"-sc_threshold", "0",
		)
	}
	if p.copyAudio {
		a = append(a, "-c:a", "copy")
	} else {
		a = append(a, "-c:a", "aac", "-b:a", "192k", "-ac", "2")
	}
	return append(a,
		"-f", "hls",
		"-hls_time", fmt.Sprint(segmentSeconds),
		// Keep every segment and let the playlist grow ("event"): a player can start
		// as soon as the first ones exist, and the finished playlist is seekable.
		"-hls_list_size", "0",
		"-hls_playlist_type", "event",
		"-hls_segment_type", "mpegts",
		// temp_file: a file appears under its real name only when it is complete.
		"-hls_flags", "independent_segments+temp_file",
		// The name is a pattern, so a "%" in the folder name must be doubled.
		"-hls_segment_filename", strings.ReplaceAll(dir, "%", "%%")+"/seg%05d.ts",
		dir+"/index.m3u8",
	)
}
