package ffmpeg

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// encoder is an ffmpeg H.264 encoder. The software one works everywhere; the
// others use a video chip and exist only on some computers and some ffmpeg
// builds.
type encoder string

const (
	encoderX264         encoder = "libx264"
	encoderVideoToolbox encoder = "h264_videotoolbox" // Apple
	encoderNVENC        encoder = "h264_nvenc"        // NVIDIA
	encoderQSV          encoder = "h264_qsv"          // Intel
)

// args are the ffmpeg options that make an 8-bit High-profile H.264 picture, which
// every HLS device plays, with this encoder. Quality is aimed for roughly what
// the software encoder's CRF 22 gives; chips differ in how they are asked, so
// each has its own.
func (e encoder) args() []string {
	common := []string{"-profile:v", "high"}
	switch e {
	case encoderVideoToolbox:
		// VideoToolbox has no constant-quality mode that older Macs honour, so it
		// is given a bitrate that is plenty for 1080p.
		return append([]string{"-c:v", string(e), "-b:v", "6M", "-pix_fmt", "yuv420p"}, common...)
	case encoderNVENC:
		return append([]string{"-c:v", string(e), "-preset", "p4", "-rc", "vbr", "-cq", "23", "-b:v", "0", "-pix_fmt", "yuv420p"}, common...)
	case encoderQSV:
		return append([]string{"-c:v", string(e), "-preset", "veryfast", "-global_quality", "23", "-pix_fmt", "nv12"}, common...)
	default:
		return append([]string{
			"-c:v", string(encoderX264), "-preset", "veryfast", "-crf", "22",
			"-pix_fmt", "yuv420p", "-level", "4.1",
		}, common...)
	}
}

// hardwareCandidates are the chip encoders worth trying on an operating system,
// best first. Linux is not a target, so it has none.
func hardwareCandidates(goos string) []encoder {
	switch goos {
	case "darwin":
		return []encoder{encoderVideoToolbox}
	case "windows":
		return []encoder{encoderNVENC, encoderQSV}
	default:
		return nil
	}
}

// listedEncoders returns which of the candidates this ffmpeg was built with, by
// reading `ffmpeg -encoders`. A build having an encoder does not mean the
// computer has the chip: that is only found out by trying, which is why every
// conversion also has the software encoder as its last resort.
func listedEncoders(out string, candidates []encoder) []encoder {
	var have []encoder
	for _, c := range candidates {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			// "V....D h264_nvenc  NVIDIA NVENC H.264 encoder": flags, then the name.
			if len(fields) >= 2 && fields[1] == string(c) {
				have = append(have, c)
				break
			}
		}
	}
	return have
}

// detectEncoders asks ffmpeg which of the candidates it has.
func detectEncoders(ctx context.Context, ffmpegPath string, candidates []encoder) []encoder {
	if len(candidates) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders")
	hideWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}
	return listedEncoders(out.String(), candidates)
}

// encoderChain is the encoders to try for one conversion, in order. The software
// encoder is always last: it is what makes "use the chip" safe to leave on.
func encoderChain(hardware []encoder, useHardware bool) []encoder {
	if !useHardware {
		return []encoder{encoderX264}
	}
	chain := append([]encoder(nil), hardware...)
	return append(chain, encoderX264)
}
