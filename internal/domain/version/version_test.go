package version

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want Version
		bad  bool
	}{
		{in: "1.1.1", want: Version{1, 1, 1, ""}},
		{in: "v1.1.2", want: Version{1, 1, 2, ""}},
		{in: " v10.20.30 ", want: Version{10, 20, 30, ""}},
		{in: "1.2.0-rc1", want: Version{1, 2, 0, "rc1"}},
		{in: "v1.2.0-rc.1", want: Version{1, 2, 0, "rc.1"}},
		{in: "1.2.0+abc123", want: Version{1, 2, 0, ""}},
		{in: "1.2.0-rc1+abc", want: Version{1, 2, 0, "rc1"}},
		{in: "dev", bad: true},
		{in: "", bad: true},
		{in: "1.2", bad: true},
		{in: "1.2.3.4", bad: true},
		{in: "1.02.3", bad: true},
		{in: "1.-2.3", bad: true},
		{in: "a.b.c", bad: true},
		{in: "1.2.3-", bad: true},
		{in: "1.2.3-rc..1", bad: true},
		{in: "1.2.3-rc_1", bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.bad {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Parse(%q) error = %v, want ErrInvalid", tt.in, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("Parse(%q) = %+v, %v; want %+v", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.1.1", "1.1.1", 0},
		{"1.1.2", "1.1.1", 1},
		{"1.1.1", "1.1.2", -1},
		{"1.10.0", "1.9.0", 1}, // the case a string comparison gets wrong
		{"2.0.0", "1.99.99", 1},
		{"1.2.0-rc1", "1.2.0", -1},
		{"1.2.0", "1.2.0-rc1", 1},
		{"1.2.0-rc1", "1.2.0-rc2", -1},
		{"1.2.0-rc2", "1.2.0-rc10", 1}, // text fields compare as text, as semver says
		{"1.2.0-rc.2", "1.2.0-rc.10", -1},
		{"1.2.0-1", "1.2.0-alpha", -1}, // numeric below text
		{"1.2.0-alpha", "1.2.0-alpha.1", -1},
		{"1.1.1-rc1", "1.1.0", 1},
		{"v1.1.2", "1.1.2", 0},
	}
	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			a, err := Parse(tt.a)
			if err != nil {
				t.Fatal(err)
			}
			b, err := Parse(tt.b)
			if err != nil {
				t.Fatal(err)
			}
			if got := a.Compare(b); got != tt.want {
				t.Fatalf("%s.Compare(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			if got := b.Compare(a); got != -tt.want {
				t.Fatalf("%s.Compare(%s) = %d, want %d (must be antisymmetric)", tt.b, tt.a, got, -tt.want)
			}
			if got := a.Newer(b); got != (tt.want > 0) {
				t.Fatalf("%s.Newer(%s) = %v", tt.a, tt.b, got)
			}
		})
	}
}

func TestString(t *testing.T) {
	for in, want := range map[string]string{"v1.1.2": "1.1.2", "1.2.0-rc1": "1.2.0-rc1", "1.2.0+x": "1.2.0"} {
		v, err := Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := v.String(); got != want {
			t.Errorf("Parse(%q).String() = %q, want %q", in, got, want)
		}
	}
}
