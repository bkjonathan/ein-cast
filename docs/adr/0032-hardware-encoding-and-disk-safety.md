# 0032 — Conversions use the video chip when it works, and refuse to fill the disk

## Context
ADR 0018 encoded with `libx264 -preset veryfast` only and left hardware encoders "a
seam, not a requirement". Software encoding of a 1080p HEVC film uses most of a
laptop's processor and, on a modest one, runs near real time, which is slow enough
that a viewer waits and the fan runs. Almost every Mac and most Windows computers
have a chip that does this at several times real time.

Two things stop a naive "just use the chip":
- Whether a chip encoder works is not knowable from the ffmpeg build. `h264_nvenc`
  is compiled into most Windows builds and fails on a computer without an NVIDIA card;
  `h264_qsv` likewise without Intel graphics. The only test is to try.
- Chips differ in how they are asked for quality, and I could verify only one of them.

Separately, an encode of a long film writes gigabytes into the cache folder (ADR 0018
bounded the number of conversions and swept idle ones, but never looked at free space),
so a nearly full disk was filled by pressing play.

## Decision
**Hardware encoding.**
- `ffmpeg/encoders.go`: an `encoder` type with the software encoder and three chip
  encoders, each with its own argument set for "8-bit High-profile H.264 at about
  CRF 22's quality": `h264_videotoolbox` (macOS, a 6 Mbit/s target, since it has no
  quality mode older Macs honour), `h264_nvenc` (`-preset p4 -rc vbr -cq 23`) and
  `h264_qsv` (`-global_quality 23`). The scale filter, the keyframe every segment and
  the audio options are shared, and unchanged.
- **Which to try:** the candidates per operating system (macOS: VideoToolbox; Windows:
  NVENC, then QSV; others: none), intersected with what `ffmpeg -encoders` lists,
  looked up once per ffmpeg program. The chain for a conversion is those, best first,
  and **always `libx264` last**.
- **Falling back is automatic.** If ffmpeg exits with an error before the stream is
  playable, the output folder is emptied and the next encoder in the chain runs; the
  failure is logged at info level ("video encoder failed; trying another"). An
  error after playback began is reported, not retried: restarting under a viewer would
  be worse than stopping. A picture that is only copied (already 8-bit H.264) uses no
  encoder at all.
- **A setting**, "Use hardware acceleration", on by default, in Settings → ffmpeg
  (`Settings.HardwareEncoding`; absent from older files means on, like KeepAwake).
  It is read at the start of each conversion, so changing it applies to the next video
  without a restart.

**Disk safety.**
- Before ffmpeg starts, the transcoder asks the operating system how much the
  conversion folder's disk can still take (`statfs` on macOS, `GetDiskFreeSpaceEx` on
  Windows, in build-tagged files) and needs `min(source size, 8 GiB) + 1 GiB`. The
  8 GiB cap is so a 60 GB 4K remux, which becomes a 1080p stream a fraction of its
  size, is not refused on a disk that has room for the result. The 1 GiB is what the
  rest of the computer is left with.
- Refusal is `app.ErrNotEnoughDiskSpace`, answered as **`507 Insufficient Storage`**
  with the code `disk_full`, and the player page says so in words ("There isn't enough
  free disk space on the computer running EinCast…"). It is not remembered: freeing
  space and pressing play again works.
- If the free space cannot be read, the conversion goes ahead (and is logged): an
  unanswerable question is no reason to refuse a film, and ffmpeg still fails cleanly
  on a full disk.

## Consequences
- **Only VideoToolbox has run on real hardware** (Apple silicon, ffmpeg 8): a test
  converts an HEVC file with it and skips, saying so, if the chip was refused and the
  processor took over. The NVENC and QSV argument
  sets are written from the ffmpeg documentation and are covered by tests of the
  argument list and of the fallback (with a stand-in ffmpeg), but have never driven a
  GPU. If one is wrong the effect is a logged fallback to the processor, not a broken
  video, and it needs a Windows machine with each chip to find out
  ([TESTING-DEVICES.md](../TESTING-DEVICES.md)). AMD (`h264_amf`) is not included.
- **A chip that fails is tried again for the next video** (a few hundred
  milliseconds each time, plus a line in the log). Remembering a failure would also
  remember a failure that was about one odd file.
- **The free-space check is a guess** about a single conversion. Two at once, or
  something else writing to the disk meanwhile, can still fill it; ffmpeg then fails
  and the error is shown. Sweeping (ADR 0018) still deletes everything.
- On macOS, VideoToolbox's bitrate is the same for a 480p source as for 1080p. That is
  fine on a home network and wasteful through a tunnel's upload; a quality-based
  option is the improvement.
- Quality differs a little between encoders (chips are less efficient than x264 at the
  same bitrate). It is a streaming copy, not an archive.

## Alternatives rejected
- **Probe the chip once at startup with a one-frame encode** and trust the answer.
  Extra process at launch, wakes a discrete GPU, and a chip that works for a test
  frame can still refuse a particular stream, so the fallback would be needed anyway.
- **Remember which encoders failed.** See above: one odd file would turn the chip off
  for everyone until restart.
- **Let the person choose the encoder.** A choice nobody can answer ("do I have
  NVENC?"); a single on/off with a safe fallback is enough.
- **Re-check free space while converting** and stop early. ffmpeg's own error on a full
  disk is clean, and a periodic check is code for a case the up-front check mostly
  prevents.
- **Estimate the output from the duration and bitrate.** More accurate, but it needs
  the probe before the check, and the check then cannot refuse before any process
  starts.
