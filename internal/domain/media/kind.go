package media

import (
	"fmt"
	"path"
	"strings"
)

// Kind is the broad category of a media file.
type Kind string

// The kinds EinCast serves.
const (
	KindVideo Kind = "video"
	KindAudio Kind = "audio"
	KindImage Kind = "image"
)

// DetectKind classifies a file by its extension (case-insensitive). It returns
// ErrUnsupported for anything EinCast does not serve.
//
// name may be a bare file name or a slash-separated path. Content sniffing is
// deliberately avoided: it would mean reading every file during a scan.
func DetectKind(name string) (Kind, error) {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".mp4", ".m4v", ".mkv", ".mov", ".avi", ".webm", ".wmv", ".mpg", ".mpeg",
		".ts", ".flv", ".3gp", ".ogv":
		return KindVideo, nil
	case ".mp3", ".m4a", ".aac", ".flac", ".wav", ".ogg", ".oga", ".opus", ".wma",
		".aiff", ".aif":
		return KindAudio, nil
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".heic", ".avif", ".tif", ".tiff":
		return KindImage, nil
	default:
		return "", fmt.Errorf("detect kind of %q: %w", name, ErrUnsupported)
	}
}
