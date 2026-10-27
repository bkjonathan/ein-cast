package media

import (
	"path"
	"slices"
	"strings"
	"time"
)

// Artwork is a picture that belongs to a video: a poster found next to it.
type Artwork struct {
	// Path is the absolute location on disk. Like Item.Path it never leaves the
	// server; clients get the picture through /thumb/{id}.
	Path string
	// Size and ModTime identify the exact file, so replacing it makes a new
	// thumbnail.
	Size    int64
	ModTime time.Time
}

// Present reports whether the item has artwork.
func (a Artwork) Present() bool { return a.Path != "" }

// artworkExts are the picture types looked for, most preferred first.
var artworkExts = []string{".jpg", ".jpeg", ".png"}

// folderArtwork are the names that stand for the whole folder.
var folderArtwork = []string{"poster", "folder", "cover"}

// IsArtworkName reports whether a file name could be a poster: a picture whose
// name is one of the conventional ones, or ends in -poster. It says nothing about
// whether the picture belongs to a video; see MatchArtwork.
func IsArtworkName(name string) bool {
	stem, ok := artworkStem(name)
	if !ok {
		return false
	}
	return slices.Contains(folderArtwork, stem) || strings.HasSuffix(stem, "-poster")
}

func artworkStem(name string) (string, bool) {
	ext := strings.ToLower(path.Ext(name))
	if !slices.Contains(artworkExts, ext) {
		return "", false
	}
	return strings.ToLower(strings.TrimSuffix(name, path.Ext(name))), true
}

// MatchArtwork picks the poster for the video called videoName from the file
// names in its folder. In order of preference:
//
//  1. <video>-poster.jpg  (Kodi's name for a movie's poster)
//  2. <video>.jpg         (a picture named exactly like the video)
//  3. poster.jpg, folder.jpg, cover.jpg, only when folderLevel is true
//
// folderLevel says the folder's poster belongs to this video: the folder holds one
// video, or the video is an episode of the series the folder is about. In a
// folder of unrelated movies, poster.jpg belongs to none of them.
//
// Within a rule .jpg wins over .jpeg and .png, and a name that differs only in
// case still matches. The file name is returned as it is on disk.
func MatchArtwork(videoName string, folder []string, folderLevel bool) (string, bool) {
	stem := strings.ToLower(strings.TrimSuffix(videoName, path.Ext(videoName)))
	for _, want := range []string{stem + "-poster", stem} {
		if name, ok := pickArtwork(want, folder); ok {
			return name, true
		}
	}
	if folderLevel {
		return MatchFolderArtwork(folder)
	}
	return "", false
}

// MatchFolderArtwork picks the picture that stands for a whole folder: poster,
// then folder, then cover. It is what a series' folder offers its episodes.
func MatchFolderArtwork(folder []string) (string, bool) {
	for _, want := range folderArtwork {
		if name, ok := pickArtwork(want, folder); ok {
			return name, true
		}
	}
	return "", false
}

// pickArtwork finds the picture whose name (without extension) is stem, trying
// the extensions in order of preference.
func pickArtwork(stem string, folder []string) (string, bool) {
	for _, ext := range artworkExts {
		for _, name := range folder {
			if strings.ToLower(name) == stem+ext {
				return name, true
			}
		}
	}
	return "", false
}

// IsSeasonFolder reports whether a folder name stands for one season of a series
// ("Season 2", "S02"), so artwork and the series name are looked for above it.
func IsSeasonFolder(name string) bool {
	return seasonFolder.MatchString(strings.TrimSpace(normalize(name)))
}
