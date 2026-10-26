package app

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestSecret_NeverPrintsItsValue(t *testing.T) {
	const value = "hunter2-hunter2"
	s := Secret(value)

	var text, json bytes.Buffer
	slog.New(slog.NewTextHandler(&text, nil)).Info("x", slog.Any("pin", s), "also", s)
	slog.New(slog.NewJSONHandler(&json, nil)).Info("x", slog.Any("pin", s), slog.Group("g", slog.Any("pin", s)))

	outputs := map[string]string{
		"text handler": text.String(),
		"json handler": json.String(),
		"%s":           fmt.Sprintf("%s", s),
		"%v":           fmt.Sprintf("%v", s),
		"%+v":          fmt.Sprintf("%+v", s),
		"%#v":          fmt.Sprintf("%#v", s),
		"%q":           fmt.Sprintf("%q", s),
		"in a struct":  fmt.Sprintf("%+v", struct{ PIN Secret }{s}),
	}
	for name, out := range outputs {
		if strings.Contains(out, value) {
			t.Errorf("%s leaked the value: %s", name, out)
		}
		if !strings.Contains(out, redacted) {
			t.Errorf("%s shows no placeholder: %s", name, out)
		}
	}
}

func TestPublicURL_Host(t *testing.T) {
	cases := map[PublicURL]string{
		"https://abc.trycloudflare.com":          "abc.trycloudflare.com",
		"https://ABC.trycloudflare.com/":         "abc.trycloudflare.com",
		"https://abc.trycloudflare.com./x":       "abc.trycloudflare.com",
		"http://den.tail1234.ts.net:8765":        "den.tail1234.ts.net:8765",
		"http://100.101.102.103:8765":            "100.101.102.103:8765",
		"https://user:pass@abc.example.com/path": "abc.example.com",
		"":                                       "",
		"just-a-word":                            "",
		"%zz":                                    "",
		"https://":                               "",
	}
	for u, want := range cases {
		if got := u.Host(); got != want {
			t.Errorf("PublicURL(%q).Host() = %q, want %q", u, got, want)
		}
	}
}
