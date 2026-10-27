// Package profile models who is watching: a name, a colour, and what that person
// may see. It is pure: no PIN hashing, no cookies, no storage (ADR 0027).
package profile

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"

	"eincast/internal/domain/media"
)

// ID names a profile. It is random and never reused, so a credential issued for a
// deleted profile cannot match a later one.
type ID string

// EveryoneID is the profile every install has: no restriction, no PIN.
const EveryoneID ID = "everyone"

// Colors are the avatar colours a profile may use. The UI draws them; the server
// only checks the name, so a hand-edited file cannot put anything else on a page.
var Colors = []string{"blue", "green", "red", "orange", "purple", "pink", "teal", "gray"}

// Limits keep a profile small enough to read and to store.
const (
	maxNameRunes    = 32
	maxPatternRunes = 64
	maxPatterns     = 20
	maxLibraries    = 200
)

// ErrInvalid is wrapped by every reason a profile or PIN is refused. The message
// is for the person who typed it.
var ErrInvalid = errors.New("invalid profile")

// Profile is one person's view of the library.
type Profile struct {
	ID    ID
	Name  string
	Color string

	// AllLibraries lets the profile see every library, including ones added later.
	// When false, only the libraries listed in Libraries.
	AllLibraries bool
	Libraries    []media.LibraryID

	// Kinds are the kinds of file it may see. Empty means all of them.
	Kinds []media.Kind

	// HiddenFolders hide every file below a folder whose name matches one of them:
	// plain names, with * and ? as wildcards, compared ignoring case.
	HiddenFolders []string
}

// Everyone returns the unrestricted default profile.
func Everyone() Profile {
	return Profile{ID: EveryoneID, Name: "Everyone", Color: "blue", AllLibraries: true}
}

// Allows is the one rule for what a profile may see. An item is shown only if its
// library, its kind and its folders are all allowed.
func (p Profile) Allows(it media.Item) bool {
	if !p.AllLibraries && !slices.Contains(p.Libraries, it.LibraryID) {
		return false
	}
	if len(p.Kinds) > 0 && !slices.Contains(p.Kinds, it.Kind) {
		return false
	}
	return !p.hides(it.RelPath)
}

// hides reports whether any folder on the way to the file matches a hidden name.
// The file's own name does not count: this hides folders, not titles.
func (p Profile) hides(relPath string) bool {
	if len(p.HiddenFolders) == 0 {
		return false
	}
	dir := path.Dir(relPath)
	if dir == "." || dir == "/" {
		return false
	}
	for _, seg := range strings.Split(dir, "/") {
		seg = strings.ToLower(seg)
		for _, pattern := range p.HiddenFolders {
			if ok, err := path.Match(strings.ToLower(pattern), seg); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// Validate reports why a profile cannot be saved, in words for the person. Name and
// patterns are expected to be trimmed already.
func (p Profile) Validate() error {
	if n := len([]rune(p.Name)); n == 0 || n > maxNameRunes {
		return fmt.Errorf("%w: the name must be 1 to %d characters", ErrInvalid, maxNameRunes)
	}
	if hasControl(p.Name) {
		return fmt.Errorf("%w: the name has a character that cannot be shown", ErrInvalid)
	}
	if !slices.Contains(Colors, p.Color) {
		return fmt.Errorf("%w: choose one of the offered colours", ErrInvalid)
	}
	if len(p.Libraries) > maxLibraries {
		return fmt.Errorf("%w: too many libraries", ErrInvalid)
	}
	seen := map[media.Kind]bool{}
	for _, k := range p.Kinds {
		if k != media.KindVideo && k != media.KindAudio && k != media.KindImage {
			return fmt.Errorf("%w: unknown kind %q", ErrInvalid, k)
		}
		if seen[k] {
			return fmt.Errorf("%w: kind %q is listed twice", ErrInvalid, k)
		}
		seen[k] = true
	}
	if len(p.HiddenFolders) > maxPatterns {
		return fmt.Errorf("%w: at most %d hidden folder names", ErrInvalid, maxPatterns)
	}
	for _, pat := range p.HiddenFolders {
		if err := validatePattern(pat); err != nil {
			return err
		}
	}
	return nil
}

func validatePattern(pat string) error {
	if n := len([]rune(pat)); n == 0 || n > maxPatternRunes {
		return fmt.Errorf("%w: a hidden folder name must be 1 to %d characters", ErrInvalid, maxPatternRunes)
	}
	if strings.ContainsAny(pat, `/\`) || hasControl(pat) {
		return fmt.Errorf("%w: %q is a folder name, so it cannot contain a slash", ErrInvalid, pat)
	}
	if _, err := path.Match(pat, ""); err != nil {
		return fmt.Errorf("%w: %q is not a valid name (a [ is not closed?)", ErrInvalid, pat)
	}
	return nil
}

func hasControl(s string) bool { return strings.IndexFunc(s, unicode.IsControl) >= 0 }

// ValidatePIN accepts four to six digits.
func ValidatePIN(pin string) error {
	if len(pin) < 4 || len(pin) > 6 {
		return fmt.Errorf("%w: a PIN is 4 to 6 digits", ErrInvalid)
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return fmt.Errorf("%w: a PIN is digits only", ErrInvalid)
		}
	}
	return nil
}

// Clean trims the text fields, so what is validated is what is stored.
func (p Profile) Clean() Profile {
	p.Name = strings.TrimSpace(p.Name)
	folders := make([]string, 0, len(p.HiddenFolders))
	for _, f := range p.HiddenFolders {
		if f = strings.TrimSpace(f); f != "" {
			folders = append(folders, f)
		}
	}
	p.HiddenFolders = folders
	p.Libraries = slices.Clone(p.Libraries)
	p.Kinds = slices.Clone(p.Kinds)
	return p
}
