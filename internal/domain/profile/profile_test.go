package profile

import (
	"errors"
	"strings"
	"testing"

	"eincast/internal/domain/media"
)

func item(lib, rel string, kind media.Kind) media.Item {
	return media.Item{ID: media.NewID(media.LibraryID(lib), rel), LibraryID: media.LibraryID(lib), RelPath: rel, Kind: kind}
}

func TestAllows(t *testing.T) {
	tests := []struct {
		name string
		p    Profile
		it   media.Item
		want bool
	}{
		{"everyone sees everything", Everyone(), item("L1", "a/b.mkv", media.KindVideo), true},
		{"a listed library", Profile{Libraries: []media.LibraryID{"L1"}}, item("L1", "a.mkv", media.KindVideo), true},
		{"an unlisted library", Profile{Libraries: []media.LibraryID{"L1"}}, item("L2", "a.mkv", media.KindVideo), false},
		{"no libraries listed and not all: nothing", Profile{}, item("L1", "a.mkv", media.KindVideo), false},
		{"all libraries includes ones added later", Profile{AllLibraries: true}, item("NEW", "a.mkv", media.KindVideo), true},
		{"a listed kind", Profile{AllLibraries: true, Kinds: []media.Kind{media.KindVideo}}, item("L1", "a.mkv", media.KindVideo), true},
		{"an unlisted kind", Profile{AllLibraries: true, Kinds: []media.Kind{media.KindVideo}}, item("L1", "a.mp3", media.KindAudio), false},
		{"no kinds listed means all", Profile{AllLibraries: true}, item("L1", "a.mp3", media.KindAudio), true},
		{"a hidden folder", Profile{AllLibraries: true, HiddenFolders: []string{"Adults"}}, item("L1", "Adults/a.mkv", media.KindVideo), false},
		{"a hidden folder, deeper down", Profile{AllLibraries: true, HiddenFolders: []string{"adults"}}, item("L1", "Films/ADULTS/Drama/a.mkv", media.KindVideo), false},
		{"another folder is fine", Profile{AllLibraries: true, HiddenFolders: []string{"Adults"}}, item("L1", "Kids/a.mkv", media.KindVideo), true},
		{"a wildcard", Profile{AllLibraries: true, HiddenFolders: []string{"18*"}}, item("L1", "18+ Films/a.mkv", media.KindVideo), false},
		{"a pattern must match the whole folder name", Profile{AllLibraries: true, HiddenFolders: []string{"Adult"}}, item("L1", "Adults/a.mkv", media.KindVideo), true},
		{"a file named like a hidden folder is not hidden", Profile{AllLibraries: true, HiddenFolders: []string{"Adults"}}, item("L1", "Adults", media.KindVideo), true},
		{"a top-level file has no folder to hide", Profile{AllLibraries: true, HiddenFolders: []string{"*"}}, item("L1", "a.mkv", media.KindVideo), true},
		{"a broken pattern hides nothing rather than everything", Profile{AllLibraries: true, HiddenFolders: []string{"[x"}}, item("L1", "x/a.mkv", media.KindVideo), true},
		{"all three must allow", Profile{Libraries: []media.LibraryID{"L1"}, Kinds: []media.Kind{media.KindVideo}, HiddenFolders: []string{"x"}}, item("L1", "x/a.mkv", media.KindVideo), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Allows(tt.it); got != tt.want {
				t.Errorf("Allows() = %v, want %v", got, tt.want)
			}
		})
	}
}

func valid() Profile { return Profile{ID: "x", Name: "Kids", Color: "green", AllLibraries: true} }

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Profile)
		bad    string // part of the message; "" means valid
	}{
		{"valid", func(*Profile) {}, ""},
		{"empty name", func(p *Profile) { p.Name = "" }, "name"},
		{"long name", func(p *Profile) { p.Name = strings.Repeat("x", 33) }, "name"},
		{"control character in the name", func(p *Profile) { p.Name = "a\x00b" }, "name"},
		{"unknown colour", func(p *Profile) { p.Color = "<script>" }, "colour"},
		{"unknown kind", func(p *Profile) { p.Kinds = []media.Kind{"book"} }, "kind"},
		{"duplicate kind", func(p *Profile) { p.Kinds = []media.Kind{media.KindVideo, media.KindVideo} }, "twice"},
		{"too many hidden names", func(p *Profile) { p.HiddenFolders = make([]string, 21) }, "at most"},
		{"an empty hidden name", func(p *Profile) { p.HiddenFolders = []string{""} }, "hidden folder name"},
		{"a slash in a hidden name", func(p *Profile) { p.HiddenFolders = []string{"a/b"} }, "slash"},
		{"a backslash in a hidden name", func(p *Profile) { p.HiddenFolders = []string{`a\b`} }, "slash"},
		{"an unclosed bracket", func(p *Profile) { p.HiddenFolders = []string{"[abc"} }, "not a valid name"},
		{"a wildcard is fine", func(p *Profile) { p.HiddenFolders = []string{"18*", "?x"} }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid()
			tt.change(&p)
			err := p.Validate()
			if tt.bad == "" {
				if err != nil {
					t.Fatalf("Validate() = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.bad) {
				t.Errorf("Validate() = %v, want an invalid-profile error mentioning %q", err, tt.bad)
			}
		})
	}
}

func TestValidatePIN(t *testing.T) {
	for pin, ok := range map[string]bool{
		"1234": true, "123456": true, "0000": true,
		"123": false, "1234567": false, "12a4": false, "": false, "12 4": false, "١٢٣٤": false,
	} {
		if err := ValidatePIN(pin); (err == nil) != ok {
			t.Errorf("ValidatePIN(%q) = %v, want ok=%v", pin, err, ok)
		}
	}
}

func TestClean(t *testing.T) {
	p := Profile{Name: "  Kids  ", HiddenFolders: []string{" Adults ", "", "  "}, Libraries: []media.LibraryID{"a"}}
	got := p.Clean()
	if got.Name != "Kids" || len(got.HiddenFolders) != 1 || got.HiddenFolders[0] != "Adults" {
		t.Errorf("Clean() = %+v", got)
	}
	got.Libraries[0] = "changed"
	if p.Libraries[0] != "a" {
		t.Error("Clean() shares its slices with the original")
	}
}

func TestEveryone(t *testing.T) {
	e := Everyone()
	if e.ID != EveryoneID || !e.AllLibraries || len(e.Kinds) != 0 || len(e.HiddenFolders) != 0 {
		t.Errorf("Everyone() = %+v", e)
	}
	if err := e.Validate(); err != nil {
		t.Errorf("the default profile is invalid: %v", err)
	}
}
