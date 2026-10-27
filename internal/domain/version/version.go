// Package version reads and compares release numbers such as "v1.1.2" or
// "1.2.0-rc1", following the precedence rules of semantic versioning. It exists
// so "is there a newer EinCast?" is one tested function and not a string
// comparison, where "1.10.0" sorts before "1.9.0".
package version

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalid means a string is not a release number.
var ErrInvalid = errors.New("not a version number")

// Version is a release number: three numbers and an optional pre-release tag.
type Version struct {
	Major, Minor, Patch int
	// Pre is what follows the hyphen ("rc1"); empty for a final release.
	Pre string
}

// Parse reads "1.2.3", "v1.2.3" or "1.2.3-rc1". Build metadata ("+abc") is
// dropped, since it never takes part in precedence. Anything else, including a
// development build's "dev", is ErrInvalid.
func Parse(s string) (Version, error) {
	orig := s
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	s, _, _ = strings.Cut(s, "+")
	core, pre, hasPre := strings.Cut(s, "-")
	if hasPre && !validPre(pre) {
		return Version{}, fmt.Errorf("%w: %q", ErrInvalid, orig)
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("%w: %q", ErrInvalid, orig)
	}
	var nums [3]int
	for i, p := range parts {
		n, ok := number(p)
		if !ok {
			return Version{}, fmt.Errorf("%w: %q", ErrInvalid, orig)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Pre: pre}, nil
}

// String is the version without a leading "v".
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare returns -1, 0 or 1 as v is older than, the same as, or newer than o.
// A pre-release is older than the release it leads up to: 1.2.0-rc1 < 1.2.0.
func (v Version) Compare(o Version) int {
	for _, d := range [3]int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		if d != 0 {
			return sign(d)
		}
	}
	return comparePre(v.Pre, o.Pre)
}

// Newer reports whether v is a later release than o.
func (v Version) Newer(o Version) bool { return v.Compare(o) > 0 }

// comparePre orders pre-release tags field by field, as semver does: numeric
// fields by value and below text fields, text fields as text, and a tag that is
// a prefix of another below it.
func comparePre(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1 // a final release is newer than any pre-release of it
	case b == "":
		return -1
	}
	af, bf := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(af) && i < len(bf); i++ {
		if c := comparePreField(af[i], bf[i]); c != 0 {
			return c
		}
	}
	return sign(len(af) - len(bf))
}

func comparePreField(a, b string) int {
	an, aNum := number(a)
	bn, bNum := number(b)
	switch {
	case aNum && bNum:
		return sign(an - bn)
	case aNum:
		return -1
	case bNum:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// number reads a non-negative decimal without a leading zero ("0" is fine).
func number(s string) (int, bool) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// validPre accepts dot-separated fields of letters, digits and hyphens.
func validPre(s string) bool {
	if s == "" {
		return false
	}
	for _, f := range strings.Split(s, ".") {
		if f == "" {
			return false
		}
		for _, r := range f {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
				return false
			}
		}
	}
	return true
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
