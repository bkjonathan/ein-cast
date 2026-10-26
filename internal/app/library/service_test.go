package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

func TestAdd_ScansAndRemembersTheFolder(t *testing.T) {
	r := newRig()
	r.sc.set("/media/films", file("a.mkv", 10), file("b.mp4", 5))

	st, err := r.svc.Add(context.Background(), "/media/films")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	lib := media.NewLibrary("/media/films")
	if st.Library != lib || st.Stats.Total != 2 || st.Stats.TotalSize != 15 || st.Scanning || st.Error != "" {
		t.Errorf("Add() = %+v", st)
	}
	if lib.Name != "films" {
		t.Errorf("a library is named after its folder, got %q", lib.Name)
	}
	if got := r.repo.Items(lib.ID); len(got) != 2 || got[0].LibraryID != lib.ID || got[0].Root != "/media/films" {
		t.Errorf("repo items = %+v, want the two items stamped with their library", got)
	}
	if got := r.mem.lastSave(); len(got) != 1 || got[0] != lib {
		t.Errorf("remembered %+v, want [%+v]", got, lib)
	}
}

func TestAdd_EventsComeInTheRightOrder(t *testing.T) {
	r := newRig()
	r.sc.set("/media", file("a.mkv", 1))
	if _, err := r.svc.Add(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}

	// The library appears (scanning), its contents arrive, then scanning ends: a
	// UI that heard "done" first would briefly show an empty library.
	if got := strings.Join(r.ev.kinds(), ","); got != "status,scanned,status" {
		t.Fatalf("events = %s, want status,scanned,status", got)
	}
	first := r.ev.statuses()[0].Libraries
	if len(first) != 1 || !first[0].Scanning || first[0].Stats.Total != 0 {
		t.Errorf("first status = %+v, want the library listed and scanning", first)
	}
	last := r.ev.statuses()[1].Libraries
	if len(last) != 1 || last[0].Scanning || last[0].Stats.Total != 1 {
		t.Errorf("last status = %+v", last)
	}
}

func TestAdd_Refusals(t *testing.T) {
	r := newRig()
	if _, err := r.svc.Add(context.Background(), "/media/films"); err != nil {
		t.Fatal(err)
	}
	before := r.sc.scans()

	tests := []struct {
		name string
		path string
		want error
	}{
		{"the same folder", "/media/films", ErrLibraryExists},
		{"the same folder written differently", "/media/films/", ErrLibraryExists},
		{"a folder inside a library", "/media/films/2020", ErrLibraryOverlaps},
		{"a folder holding a library", "/media", ErrLibraryOverlaps},
		{"a folder that differs in case only", "/MEDIA/Films/Old", ErrLibraryOverlaps},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := r.svc.Add(context.Background(), tt.path); !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
	if r.sc.scans() != before || len(r.svc.Libraries()) != 1 {
		t.Errorf("a refused folder was scanned (%d scans) or listed (%d libraries)", r.sc.scans()-before, len(r.svc.Libraries()))
	}
	if _, err := r.svc.Add(context.Background(), "/media/films-old"); err != nil {
		t.Errorf("a sibling whose name merely starts the same is not an overlap: %v", err)
	}
	if _, err := r.svc.Add(context.Background(), "  "); err == nil {
		t.Error("an empty path was accepted")
	}
}

func TestAdd_AFailedScanLeavesNothingBehind(t *testing.T) {
	r := newRig()
	r.sc.set("/good", file("a.mkv", 1))
	_, _ = r.svc.Add(context.Background(), "/good")
	boom := fmt.Errorf("walk: %w", fs.ErrNotExist)
	r.sc.fail("/bad", boom)

	_, err := r.svc.Add(context.Background(), "/bad")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want it to wrap the cause", err)
	}
	if !strings.Contains(err.Error(), "no longer exists") || strings.Contains(err.Error(), "/bad") {
		t.Errorf("message = %q, want a sentence for the person without paths", err.Error())
	}

	libs := r.svc.Libraries()
	if len(libs) != 1 || libs[0].Root != "/good" {
		t.Errorf("libraries = %+v, want only the good one: a bad choice must not leave a broken entry", libs)
	}
	if got := r.mem.lastSave(); len(got) != 1 || got[0].Root != "/good" {
		t.Errorf("remembered %+v", got)
	}
	if r.repo.has(media.NewLibraryID("/bad")) {
		t.Error("the repository holds the failed library")
	}
	if n := len(r.svc.List()); n != 1 {
		t.Errorf("the good library lost items: %d", n)
	}
}

func TestAdd_ASettingsProblemDoesNotLoseTheScan(t *testing.T) {
	r := newRig()
	r.mem.err = errors.New("read-only disk")
	r.sc.set("/media", file("a.mkv", 1))

	if _, err := r.svc.Add(context.Background(), "/media"); err != nil {
		t.Fatalf("Add() error = %v; a settings problem must not fail the add", err)
	}
	if len(r.svc.List()) != 1 {
		t.Error("the library was not loaded")
	}
}

func TestScanFailureIsExplainedInTheState(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		contain string
	}{
		{"folder vanished", fmt.Errorf("stat root: %w", fs.ErrNotExist), "no longer exists"},
		{"no permission", fmt.Errorf("open: %w", fs.ErrPermission), "isn't allowed"},
		{"cancelled", context.Canceled, "cancelled"},
		{"anything else", errors.New("disk exploded at /secret/path"), "couldn't be scanned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := media.NewLibrary("/media")
			r := newRig(lib)
			r.sc.fail("/media", tt.err)

			if err := r.svc.Rescan(context.Background(), lib.ID); !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want it to wrap %v", err, tt.err)
			}
			st, _ := r.stateOf(lib.ID)
			if st.Scanning || !strings.Contains(st.Error, tt.contain) {
				t.Errorf("state = %+v, want an error containing %q", st, tt.contain)
			}
			if strings.Contains(st.Error, "/secret") || strings.Contains(st.Error, "exploded") {
				t.Errorf("the error leaks system detail: %q", st.Error)
			}
			last := r.ev.statuses()[len(r.ev.statuses())-1].Libraries[0]
			if last.Error != st.Error || last.Scanning {
				t.Errorf("last published %+v differs from the state %+v", last, st)
			}
		})
	}
}

func TestRescan_FindsChangesAndKeepsTheOldContentsOnFailure(t *testing.T) {
	lib := media.NewLibrary("/media")
	r := newRig(lib)
	r.sc.set("/media", file("a.mkv", 1))
	if err := r.svc.Rescan(context.Background(), lib.ID); err != nil {
		t.Fatal(err)
	}

	r.sc.set("/media", file("a.mkv", 1), file("b.mkv", 2))
	if err := r.svc.Rescan(context.Background(), lib.ID); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.stateOf(lib.ID); st.Stats.Total != 2 {
		t.Errorf("after adding a file: %+v", st)
	}

	r.sc.fail("/media", fs.ErrNotExist)
	_ = r.svc.Rescan(context.Background(), lib.ID)
	if got := r.svc.List(); len(got) != 2 {
		t.Errorf("a failed rescan changed the library: %d items", len(got))
	}

	r.sc.set("/media", file("a.mkv", 1)) // readable again: the error clears
	if err := r.svc.Rescan(context.Background(), lib.ID); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.stateOf(lib.ID); st.Error != "" || st.Stats.Total != 1 {
		t.Errorf("state = %+v, want the error cleared", st)
	}
}

func TestRescan_OnlyTouchesItsOwnLibrary(t *testing.T) {
	films, music := media.NewLibrary("/films"), media.NewLibrary("/music")
	r := newRig(films, music)
	r.sc.set("/films", file("a.mkv", 1))
	r.sc.set("/music", file("s.mp3", 1))
	_ = r.svc.Restore(context.Background())
	before := r.sc.scansOf("/music")

	r.sc.set("/films", file("a.mkv", 1), file("b.mkv", 1))
	if err := r.svc.Rescan(context.Background(), films.ID); err != nil {
		t.Fatal(err)
	}
	if r.sc.scansOf("/music") != before {
		t.Error("rescanning films scanned music too")
	}
	if got := r.repo.Items(music.ID); len(got) != 1 {
		t.Errorf("music changed: %+v", got)
	}
	if err := r.svc.Rescan(context.Background(), "nope"); !errors.Is(err, ErrNoSuchLibrary) {
		t.Errorf("unknown library: error = %v", err)
	}
}

func TestRescanAll_OneFailureDoesNotStopTheOthers(t *testing.T) {
	a, b, c := media.NewLibrary("/a"), media.NewLibrary("/b"), media.NewLibrary("/c")
	r := newRig(a, b, c)
	r.sc.set("/a", file("1.mkv", 1))
	r.sc.fail("/b", fs.ErrNotExist)
	r.sc.set("/c", file("3.mkv", 1))

	err := r.svc.RescanAll(context.Background())
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want the failure reported", err)
	}
	if got := r.sc.lastCalls(); !slices.Equal(got, []string{"/a", "/b", "/c"}) {
		t.Errorf("scanned %v, want all three in order", got)
	}
	if len(r.svc.List()) != 2 {
		t.Errorf("items = %d, want a's and c's", len(r.svc.List()))
	}
	if st, _ := r.stateOf(b.ID); st.Error == "" {
		t.Error("b's failure is not in its state")
	}
	if st, _ := r.stateOf(a.ID); st.Error != "" {
		t.Errorf("a has an error: %q", st.Error)
	}
}

func TestRemove(t *testing.T) {
	films, music := media.NewLibrary("/films"), media.NewLibrary("/music")
	r := newRig(films, music)
	r.sc.set("/films", file("a.mkv", 1))
	r.sc.set("/music", file("s.mp3", 1))
	_ = r.svc.Restore(context.Background())
	before := len(r.ev.events())

	if err := r.svc.Remove(films.ID); err != nil {
		t.Fatal(err)
	}
	if libs := r.svc.Libraries(); len(libs) != 1 || libs[0].ID != music.ID {
		t.Errorf("libraries = %+v", libs)
	}
	if got := r.svc.List(); len(got) != 1 || got[0].LibraryID != music.ID {
		t.Errorf("items = %+v, want only music's", got)
	}
	if r.repo.has(films.ID) {
		t.Error("the repository still holds the removed library")
	}
	if got := r.mem.lastSave(); len(got) != 1 || got[0].ID != music.ID {
		t.Errorf("remembered %+v", got)
	}
	if len(r.ev.events()) == before {
		t.Error("the UI was not told")
	}
	if got := r.ev.scanned()[len(r.ev.scanned())-1].Libraries; len(got) != 1 {
		t.Errorf("last LibraryScanned listed %d libraries, want 1", len(got))
	}
	if err := r.svc.Remove(films.ID); !errors.Is(err, ErrNoSuchLibrary) {
		t.Errorf("removing twice: error = %v", err)
	}
}

func TestRemove_AScanInFlightIsDiscarded(t *testing.T) {
	lib := media.NewLibrary("/media")
	r := newRig(lib)
	r.sc.set("/media", file("a.mkv", 1))
	h := newHold()
	r.sc.hold["/media"] = h

	done := make(chan error, 1)
	go func() { done <- r.svc.Rescan(context.Background(), lib.ID) }()
	<-h.entered

	if err := r.svc.Remove(lib.ID); err != nil {
		t.Fatal(err)
	}
	close(h.release)
	if err := <-done; !errors.Is(err, ErrNoSuchLibrary) {
		t.Errorf("the scan of a removed library: error = %v", err)
	}
	if r.repo.has(lib.ID) || len(r.svc.List()) != 0 {
		t.Error("a scan that finished after the library was removed put its files back")
	}
}

func TestRename(t *testing.T) {
	lib := media.NewLibrary("/media/films")
	r := newRig(lib)

	if err := r.svc.Rename(lib.ID, "  Movie night  "); err != nil {
		t.Fatal(err)
	}
	st, _ := r.stateOf(lib.ID)
	if st.Library.Name != "Movie night" || st.Library.Root != "/media/films" || st.Library.ID != lib.ID {
		t.Errorf("state = %+v: only the name changes", st.Library)
	}
	if got := r.mem.lastSave(); len(got) != 1 || got[0].Name != "Movie night" {
		t.Errorf("remembered %+v", got)
	}
	if len(r.ev.statuses()) == 0 {
		t.Error("the UI was not told")
	}

	for _, bad := range []string{"", "   ", strings.Repeat("x", 65), "a\nb", "tab\there"} {
		if err := r.svc.Rename(lib.ID, bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Rename(%q) = %v, want ErrInvalidName", bad, err)
		}
	}
	if err := r.svc.Rename("nope", "x"); !errors.Is(err, ErrNoSuchLibrary) {
		t.Errorf("unknown library: %v", err)
	}
	if st, _ := r.stateOf(lib.ID); st.Library.Name != "Movie night" {
		t.Errorf("a refused rename changed the name: %q", st.Library.Name)
	}
}

func TestRename_LongNameInCharactersNotBytes(t *testing.T) {
	lib := media.NewLibrary("/media")
	r := newRig(lib)
	if err := r.svc.Rename(lib.ID, strings.Repeat("မ", 64)); err != nil {
		t.Errorf("64 characters (192 bytes) refused: %v", err)
	}
}

func TestNewService_ListsRememberedLibrariesAtOnce(t *testing.T) {
	films, music := media.NewLibrary("/films"), media.NewLibrary("/music")
	r := newRig(films, music)

	// Before Restore scans anything the UI can already show them, so a restart is
	// not mistaken for a first run.
	got := r.svc.State()
	if len(got) != 2 || got[0].Library != films || got[1].Library != music || got[0].Scanning || got[0].Stats.Total != 0 {
		t.Fatalf("State() = %+v", got)
	}
	if r.sc.scans() != 0 {
		t.Error("constructing the service scanned something")
	}
}

func TestRestore(t *testing.T) {
	t.Run("nothing remembered does nothing", func(t *testing.T) {
		r := newRig()
		if err := r.svc.Restore(context.Background()); err != nil || r.sc.scans() != 0 {
			t.Fatalf("err=%v scans=%d, want a quiet no-op", err, r.sc.scans())
		}
	})

	t.Run("scans every remembered library", func(t *testing.T) {
		films, music := media.NewLibrary("/films"), media.NewLibrary("/music")
		r := newRig(films, music)
		r.sc.set("/films", file("a.mkv", 1))
		r.sc.set("/music", file("s.mp3", 1), file("t.mp3", 1))
		if err := r.svc.Restore(context.Background()); err != nil {
			t.Fatal(err)
		}
		if st, _ := r.stateOf(music.ID); st.Stats.Total != 2 {
			t.Errorf("music = %+v", st)
		}
		if len(r.svc.List()) != 3 {
			t.Errorf("items = %d", len(r.svc.List()))
		}
	})

	t.Run("a vanished folder is reported through its state and the others still open", func(t *testing.T) {
		gone, ok := media.NewLibrary("/gone"), media.NewLibrary("/ok")
		r := newRig(gone, ok)
		r.sc.fail("/gone", fs.ErrNotExist)
		r.sc.set("/ok", file("a.mkv", 1))

		if err := r.svc.Restore(context.Background()); err == nil {
			t.Fatal("Restore() error = nil")
		}
		if st, _ := r.stateOf(gone.ID); !strings.Contains(st.Error, "no longer exists") {
			t.Errorf("gone = %+v", st)
		}
		if st, _ := r.stateOf(ok.ID); st.Error != "" || st.Stats.Total != 1 {
			t.Errorf("ok = %+v", st)
		}
		if len(r.svc.Libraries()) != 2 {
			t.Error("a library whose folder is missing was dropped; it may be an unplugged drive")
		}
	})
}

func TestScan_OneLibraryAtATimeButLibrariesDoNotBlockEachOther(t *testing.T) {
	a, b := media.NewLibrary("/a"), media.NewLibrary("/b")
	r := newRig(a, b)
	r.sc.set("/a", file("1.mkv", 1))
	r.sc.set("/b", file("2.mkv", 1))
	h := newHold()
	r.sc.hold["/a"] = h

	done := make(chan error, 1)
	go func() { done <- r.svc.Rescan(context.Background(), a.ID) }()
	<-h.entered

	if st, _ := r.stateOf(a.ID); !st.Scanning {
		t.Error("State().Scanning = false during a scan")
	}
	if err := r.svc.Rescan(context.Background(), a.ID); !errors.Is(err, ErrScanInProgress) {
		t.Fatalf("second scan of the same library: error = %v, want ErrScanInProgress", err)
	}
	if err := r.svc.Rescan(context.Background(), b.ID); err != nil {
		t.Fatalf("scanning b while a is scanned: %v", err)
	}

	close(h.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if st, _ := r.stateOf(a.ID); st.Scanning {
		t.Error("Scanning stuck at true after the scan finished")
	}
}

func TestListAndGet_SpanEveryLibrary(t *testing.T) {
	films, music := media.NewLibrary("/films"), media.NewLibrary("/music")
	r := newRig(films, music)
	r.sc.set("/films", file("a.mkv", 1))
	r.sc.set("/music", file("s.mp3", 1))
	_ = r.svc.Restore(context.Background())

	if got := r.svc.List(); len(got) != 2 || got[0].LibraryID != films.ID || got[1].LibraryID != music.ID {
		t.Errorf("List() = %+v", got)
	}
	it, err := r.svc.Get("id:s.mp3")
	if err != nil || it.LibraryID != music.ID || it.Root != "/music" {
		t.Errorf("Get() = %+v, %v; want the item with its own library's root", it, err)
	}
	if _, err := r.svc.Get("zzz"); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Get(zzz) = %v, want ErrNotFound", err)
	}
}

func TestState_CountsPerLibrary(t *testing.T) {
	films, music := media.NewLibrary("/films"), media.NewLibrary("/music")
	r := newRig(films, music)
	r.sc.set("/films", file("a.mkv", 10))
	song := file("s.mp3", 4)
	song.Kind = media.KindAudio
	r.sc.set("/music", song, song)
	song2 := song
	song2.ID = "id:t.mp3"
	r.sc.set("/music", song, song2)
	_ = r.svc.Restore(context.Background())

	got := r.svc.State()
	if got[0].Stats.Total != 1 || got[0].Stats.Counts[media.KindVideo] != 1 ||
		got[1].Stats.Total != 2 || got[1].Stats.Counts[media.KindAudio] != 2 || got[1].Stats.TotalSize != 8 {
		t.Errorf("State() = %+v", got)
	}
	var _ []app.LibraryState = got
}
