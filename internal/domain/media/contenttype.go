package media

import (
	"mime"
	"path"
	"strings"
)

// ContentType picks a MIME type by extension. Types that operating systems
// often get wrong or omit (notably .mkv) are pinned first, so playback does not
// depend on the host's mime database.
func ContentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".mkv":
		return "video/x-matroska"
	case ".m4v":
		return "video/x-m4v"
	case ".ts":
		return "video/mp2t"
	case ".avi":
		return "video/x-msvideo"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	case ".3gp":
		return "video/3gpp"
	case ".flac":
		return "audio/flac"
	case ".opus":
		return "audio/ogg; codecs=opus"
	case ".m4a":
		return "audio/mp4"
	case ".wma":
		return "audio/x-ms-wma"
	case ".aif", ".aiff":
		return "audio/aiff"
	case ".heic":
		return "image/heic"
	case ".avif":
		return "image/avif"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}
