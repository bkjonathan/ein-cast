package dlna

import (
	"regexp"
	"slices"
	"testing"
	"time"

	"eincast/internal/domain/media"
)

// testLib is the library the test items belong to.
const testLib media.LibraryID = "testlib"

func item(rel string, kind media.Kind) media.Item {
	name := rel[lastSlash(rel)+1:]
	return media.Item{ID: media.NewID(testLib, rel), Name: name, RelPath: rel, Kind: kind, Size: 100, ModTime: time.Unix(1_700_000_000, 0)}
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func titles(nodes []*node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.title
	}
	return out
}

func TestBuildTree_ShapeAndOrder(t *testing.T) {
	items := []media.Item{
		item("shows/Show/Episode 10.mkv", media.KindVideo),
		item("shows/Show/Episode 2.mkv", media.KindVideo),
		item("shows/Show/Season 2/a.mkv", media.KindVideo),
		item("movie.mp4", media.KindVideo),
		item("Zebra.mp4", media.KindVideo),
		item("music/song.flac", media.KindAudio),
		item("pics/a.jpg", media.KindImage),
	}
	tr := buildTree(items)

	if got := titles(tr.root.children); !slices.Equal(got, []string{"Videos", "Music", "Photos"}) {
		t.Fatalf("root = %v, want the kinds in a fixed order", got)
	}
	videos := tr.byID["k-video"]
	if got := titles(videos.children); !slices.Equal(got, []string{"shows", "movie", "Zebra"}) {
		t.Errorf("Videos = %v, want folders first, then files by name", got)
	}
	show := videos.children[0].children[0]
	if show.title != "Show" {
		t.Fatalf("show = %q", show.title)
	}
	if got := titles(show.children); !slices.Equal(got, []string{"Season 2", "Episode 2", "Episode 10"}) {
		t.Errorf("Show = %v, want Episode 2 before Episode 10", got)
	}
}

func TestBuildTree_OnlyKindsWithFilesAppear(t *testing.T) {
	tr := buildTree([]media.Item{item("a.mp3", media.KindAudio)})
	if got := titles(tr.root.children); !slices.Equal(got, []string{"Music"}) {
		t.Fatalf("root = %v", got)
	}
	if _, ok := tr.byID["k-video"]; ok {
		t.Error("an empty Videos folder is registered")
	}
	empty := buildTree(nil)
	if len(empty.root.children) != 0 || len(empty.byID) != 1 {
		t.Errorf("empty library: %d children, %d nodes", len(empty.root.children), len(empty.byID))
	}
}

func TestBuildTree_ParentsAndLookups(t *testing.T) {
	it := item("tv/s1/e1.mkv", media.KindVideo)
	tr := buildTree([]media.Item{it})

	leaf := tr.byID["i-"+string(it.ID)]
	if leaf == nil || leaf.item == nil || leaf.title != "e1" {
		t.Fatalf("leaf = %+v", leaf)
	}
	for n := leaf; n.id != rootID; n = tr.byID[n.parentID] {
		if tr.byID[n.parentID] == nil {
			t.Fatalf("%s has a parent %q that is not in the tree", n.id, n.parentID)
		}
	}
	if tr.root.parentID != "-1" {
		t.Errorf("root parent = %q, want -1 as UPnP requires", tr.root.parentID)
	}
}

var idShape = regexp.MustCompile(`^(0|k-(video|audio|image)|d-[0-9a-f]{16}|i-[0-9a-f]{16})$`)

func TestBuildTree_IDsAreStableAndSayNothingAboutPaths(t *testing.T) {
	a := buildTree([]media.Item{item("tv/s1/e1.mkv", media.KindVideo)})
	b := buildTree([]media.Item{item("tv/s1/e1.mkv", media.KindVideo), item("other.mkv", media.KindVideo)})
	for id := range a.byID {
		if _, ok := b.byID[id]; !ok {
			t.Errorf("id %q changed when an unrelated file was added", id)
		}
		if !idShape.MatchString(id) {
			t.Errorf("id %q is not one of the opaque forms (no folder or file names may appear in it)", id)
		}
	}
	// The same folder name under two kinds is two folders.
	if dirID(media.KindVideo, "x") == dirID(media.KindAudio, "x") {
		t.Error("video and audio folders named x share an ID")
	}
}

func TestNaturalCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int // sign
	}{
		{"Episode 2", "Episode 10", -1},
		{"episode 10", "Episode 2", 1},
		{"a", "A", 0},
		{"file01", "file1", 0},
		{"file02", "file1", 1},
		{"abc", "abcd", -1},
		{"", "a", -1},
		{"10", "9", 1},
		{"x2y", "x2z", -1},
		{"007", "7", 0},
	}
	for _, tt := range tests {
		got := naturalCompare(tt.a, tt.b)
		if (got > 0) != (tt.want > 0) || (got < 0) != (tt.want < 0) {
			t.Errorf("naturalCompare(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
		}
	}
}
