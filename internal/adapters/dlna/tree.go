// Package dlna makes the library visible to smart TVs and other UPnP media
// players: a MediaServer description, a ContentDirectory they can browse, and
// the SSDP announcements that let them find it. The files themselves are served
// by the ordinary /stream/{id} route.
package dlna

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"slices"
	"strings"
	"unicode"

	"eincast/internal/domain/media"
)

// Object IDs. A TV remembers and sends these back, so they are stable: the
// same folder has the same ID across scans and restarts. They are opaque names
// looked up in a tree built from the library, never paths (ADR 0005).
const (
	rootID     = "0"
	kindPrefix = "k-"
	dirPrefix  = "d-"
	itemPrefix = "i-"
)

// node is one thing a TV can show: a folder or a file.
type node struct {
	id       string
	parentID string
	title    string
	children []*node // folders only
	item     *media.Item
}

func (n *node) isContainer() bool { return n.item == nil }

// tree is the library as a TV browses it: a root with one folder per kind of
// media that exists, and below it the library's own folders.
type tree struct {
	byID map[string]*node
	root *node
}

var kindFolders = []struct {
	kind  media.Kind
	title string
}{
	{media.KindVideo, "Videos"},
	{media.KindAudio, "Music"},
	{media.KindImage, "Photos"},
}

// buildTree arranges items into folders. Videos, music and photos each get a
// top-level folder (only when there is something in it), and below that the
// folders mirror the library on disk. Folders come before files, each sorted by
// name with numbers in numeric order ("Episode 2" before "Episode 10").
func buildTree(items []media.Item) *tree {
	t := &tree{byID: map[string]*node{}}
	t.root = &node{id: rootID, parentID: "-1", title: "EinCast"}
	t.byID[rootID] = t.root

	tops := map[media.Kind]*node{}
	for _, kf := range kindFolders {
		tops[kf.kind] = &node{id: kindPrefix + string(kf.kind), parentID: rootID, title: kf.title}
	}

	for i := range items {
		it := &items[i]
		top, ok := tops[it.Kind]
		if !ok {
			continue
		}
		if _, linked := t.byID[top.id]; !linked {
			t.byID[top.id] = top
			t.root.children = append(t.root.children, top)
		}
		parent := t.folderFor(top, it.Kind, path.Dir(it.RelPath))
		leaf := &node{id: itemPrefix + string(it.ID), parentID: parent.id, title: titleOf(it.Name), item: it}
		t.byID[leaf.id] = leaf
		parent.children = append(parent.children, leaf)
	}

	for _, n := range t.byID {
		sortChildren(n.children)
	}
	// The three top-level folders keep their fixed order, not alphabetical.
	order := func(n *node) int {
		for i, kf := range kindFolders {
			if n.id == kindPrefix+string(kf.kind) {
				return i
			}
		}
		return len(kindFolders)
	}
	slices.SortFunc(t.root.children, func(a, b *node) int { return order(a) - order(b) })
	return t
}

// folderFor returns the folder node for a slash path below a kind's top folder,
// creating it and the ones above it as needed. "." is the top folder itself.
func (t *tree) folderFor(top *node, kind media.Kind, dir string) *node {
	if dir == "." || dir == "" || dir == "/" {
		return top
	}
	parent := t.folderFor(top, kind, path.Dir(dir))
	id := dirID(kind, dir)
	if n, ok := t.byID[id]; ok {
		return n
	}
	n := &node{id: id, parentID: parent.id, title: path.Base(dir)}
	t.byID[id] = n
	parent.children = append(parent.children, n)
	return n
}

// dirID names a folder by a hash of its kind and its path inside the library, so
// the ID is short, stable, and says nothing about the disk.
func dirID(kind media.Kind, relDir string) string {
	sum := sha256.Sum256([]byte(string(kind) + "|" + relDir))
	return dirPrefix + hex.EncodeToString(sum[:8])
}

// titleOf is a file name as a TV should show it: without its extension.
func titleOf(name string) string {
	return strings.TrimSuffix(name, path.Ext(name))
}

// sortChildren puts folders first, then files, each in natural name order.
func sortChildren(nodes []*node) {
	slices.SortStableFunc(nodes, func(a, b *node) int {
		switch {
		case a.isContainer() && !b.isContainer():
			return -1
		case !a.isContainer() && b.isContainer():
			return 1
		}
		return naturalCompare(a.title, b.title)
	})
}

// naturalCompare orders strings ignoring case, with runs of digits compared as
// numbers, so "Episode 2" sorts before "Episode 10".
func naturalCompare(a, b string) int {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si, sj := i, j
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na, nb := strings.TrimLeft(string(ra[si:i]), "0"), strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) - len(nb)
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			continue
		}
		if ra[i] != rb[j] {
			if ra[i] < rb[j] {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	return (len(ra) - i) - (len(rb) - j)
}
