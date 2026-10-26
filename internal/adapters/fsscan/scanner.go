// Package fsscan is the filesystem implementation of app.Scanner.
package fsscan

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"eincast/internal/domain/media"
)

// Scanner walks a directory tree and lists the media files in it.
type Scanner struct {
	log *slog.Logger
}

// New returns a Scanner that logs skipped entries to log.
func New(log *slog.Logger) *Scanner {
	return &Scanner{log: log.With(slog.String("source", "library"))}
}

// Scan returns every supported media file under root in walk order (lexical
// within each directory).
//
// It deliberately skips:
//   - hidden entries and well-known OS/NAS metadata (see media.Ignored);
//   - symlinks, so a link inside the library cannot expose files outside it;
//   - files with an unsupported extension.
//
// Subtitle files (.srt, .vtt) are not items. They are attached to the video
// they belong to (see media.MatchSubtitles). Neither are pictures that are a
// video's poster (see media.MatchArtwork); a photo that merely sits near a video
// stays a photo. A Kodi-style .nfo file next to a video corrects what its name
// says (see media.ParseNFO).
//
// An unreadable subdirectory is logged and skipped rather than failing the
// whole scan; an unusable root is an error.
func (s *Scanner) Scan(ctx context.Context, lib media.Library) ([]media.Item, error) {
	absRoot, err := filepath.Abs(lib.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", lib.Root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %q is not a directory", absRoot)
	}

	var items []media.Item
	folders := make(map[string]*folder) // folder → the side files in it
	walk := func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if path == absRoot {
				return walkErr
			}
			s.log.Warn("skipping unreadable entry",
				slog.String("path", path), slog.String("error", walkErr.Error()))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == absRoot {
			return nil
		}
		if media.Ignored(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // directories to descend into, symlinks, devices, sockets
		}

		if media.IsSubtitleFile(d.Name()) {
			f := folderOf(folders, filepath.Dir(path))
			f.subtitles = append(f.subtitles, d.Name())
			return nil
		}
		if media.IsNFOFile(d.Name()) {
			f := folderOf(folders, filepath.Dir(path))
			f.nfos = append(f.nfos, d.Name())
			return nil
		}

		item, ok := s.itemFor(lib.ID, absRoot, path, d)
		if ok {
			if item.Kind == media.KindImage && isArtworkFile(d.Name()) {
				f := folderOf(folders, filepath.Dir(path))
				f.images = append(f.images, d.Name())
			}
			items = append(items, item)
		}
		return nil
	}

	if err := filepath.WalkDir(absRoot, walk); err != nil {
		return nil, fmt.Errorf("walk %q: %w", absRoot, err)
	}
	return s.enrich(absRoot, items, folders), nil
}

// folder holds the side files found in one folder while walking: the files that
// may belong to the videos next to them.
type folder struct {
	subtitles []string
	images    []string // pictures that could be a poster
	nfos      []string
}

func folderOf(folders map[string]*folder, dir string) *folder {
	f := folders[dir]
	if f == nil {
		f = &folder{}
		folders[dir] = f
	}
	return f
}

// isArtworkFile is true for a picture a video could use as its poster: any
// jpg or png, since "<video>.jpg" has no fixed name. Whether it is one is decided
// per video.
func isArtworkFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png"
}

// enrich adds to each video what its side files and name say: subtitles, a title
// and year or the series it belongs to, a description from a .nfo file, a poster.
// A picture that became a video's poster is removed from the items, so a poster is
// not also listed as a photo.
func (s *Scanner) enrich(root string, items []media.Item, folders map[string]*folder) []media.Item {
	videos := make(map[string]int) // folder → how many videos it holds
	for _, it := range items {
		if it.Kind == media.KindVideo {
			videos[filepath.Dir(it.Path)]++
		}
	}

	claimed := make(map[string]struct{}) // posters in use
	for i := range items {
		it := &items[i]
		if it.Kind != media.KindVideo {
			continue
		}
		dir := filepath.Dir(it.Path)
		f := folders[dir]
		if f == nil {
			f = &folder{}
		}

		it.Subtitles = attachSubtitles(root, *it, f.subtitles)
		it.Meta = media.ParseMeta(it.RelPath)
		if nfo, ok := s.readNFO(it, f.nfos, videos[dir] == 1); ok {
			it.Meta = it.Meta.Apply(nfo)
		}
		it.Artwork = s.findArtwork(root, dir, *it, folders, videos[dir] == 1)
		if it.Artwork.Present() {
			claimed[it.Artwork.Path] = struct{}{}
		}
	}

	if len(claimed) == 0 {
		return items
	}
	kept := items[:0]
	for _, it := range items {
		if _, isPoster := claimed[it.Path]; !isPoster {
			kept = append(kept, it)
		}
	}
	return kept
}

// readNFO reads the .nfo that belongs to a video: Movie.nfo next to Movie.mkv, or
// movie.nfo when the folder holds only this video. An unreadable or nonsensical
// file is skipped quietly; the name's guess stands.
func (s *Scanner) readNFO(it *media.Item, nfos []string, alone bool) (media.NFO, bool) {
	if len(nfos) == 0 {
		return media.NFO{}, false
	}
	stem := strings.ToLower(strings.TrimSuffix(it.Name, filepath.Ext(it.Name)))
	var name string
	for _, n := range nfos {
		if strings.ToLower(strings.TrimSuffix(n, filepath.Ext(n))) == stem {
			name = n
			break
		}
		if alone && strings.EqualFold(n, "movie.nfo") {
			name = n
		}
	}
	if name == "" {
		return media.NFO{}, false
	}

	path := filepath.Join(filepath.Dir(it.Path), name)
	data, err := readLimited(path, media.MaxNFOSize)
	if err != nil {
		s.log.Debug("skipping unreadable nfo", slog.String("path", path), slog.String("error", err.Error()))
		return media.NFO{}, false
	}
	nfo, err := media.ParseNFO(data)
	if err != nil {
		s.log.Debug("skipping unusable nfo", slog.String("path", path), slog.String("error", err.Error()))
		return media.NFO{}, false
	}
	return nfo, true
}

// readLimited reads a file, refusing one longer than max bytes. The scan walked
// to this path itself and only follows regular files, so it needs no root check.
func readLimited(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("larger than %d bytes", max)
	}
	return data, nil
}

// findArtwork looks for a poster beside the video, and for an episode in a season
// folder, in the series folder above it.
func (s *Scanner) findArtwork(root, dir string, it media.Item, folders map[string]*folder, alone bool) media.Artwork {
	level := alone || it.Meta.IsEpisode()
	if f := folders[dir]; f != nil {
		if name, ok := media.MatchArtwork(it.Name, f.images, level); ok {
			return s.artwork(filepath.Join(dir, name))
		}
	}
	if it.Meta.IsEpisode() && dir != root && media.IsSeasonFolder(filepath.Base(dir)) {
		if f := folders[filepath.Dir(dir)]; f != nil {
			// Only the series' own poster names: "<episode>.jpg" is not up here.
			if name, ok := media.MatchFolderArtwork(f.images); ok {
				return s.artwork(filepath.Join(filepath.Dir(dir), name))
			}
		}
	}
	return media.Artwork{}
}

func (s *Scanner) artwork(path string) media.Artwork {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return media.Artwork{}
	}
	return media.Artwork{Path: path, Size: info.Size(), ModTime: info.ModTime()}
}

// attachSubtitles returns the sidecar subtitles of a video, given the subtitle
// files found in its folder.
func attachSubtitles(root string, video media.Item, folder []string) []media.Subtitle {
	matches := media.MatchSubtitles(video.Name, folder)
	if len(matches) == 0 {
		return nil
	}
	dir := filepath.Dir(video.Path)
	out := make([]media.Subtitle, len(matches))
	for i, m := range matches {
		path := filepath.Join(dir, m.Name)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = m.Name // cannot happen for a path found under root
		}
		out[i] = media.Subtitle{Lang: m.Lang, Label: m.Label, RelPath: filepath.ToSlash(rel), Path: path}
	}
	return out
}

// itemFor builds the media.Item for a regular file, or reports false if the
// file is not servable.
func (s *Scanner) itemFor(libID media.LibraryID, root, path string, d fs.DirEntry) (media.Item, bool) {
	kind, err := media.DetectKind(d.Name())
	if err != nil {
		return media.Item{}, false // unsupported type: expected, not worth a log line
	}
	info, err := d.Info()
	if err != nil {
		s.log.Warn("skipping file without stat info",
			slog.String("path", path), slog.String("error", err.Error()))
		return media.Item{}, false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return media.Item{}, false // cannot happen for a path found under root
	}
	// IDs hash the slash form so they match across macOS and Windows.
	rel = filepath.ToSlash(rel)

	return media.Item{
		ID:        media.NewID(libID, rel),
		LibraryID: libID,
		Root:      root,
		Name:      d.Name(),
		RelPath:   rel,
		Path:      path,
		Kind:      kind,
		Size:      info.Size(),
		ModTime:   info.ModTime(),
	}, true
}
