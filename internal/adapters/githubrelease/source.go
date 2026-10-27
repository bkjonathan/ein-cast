// Package githubrelease reads the newest EinCast release from the GitHub
// Releases API. It implements app.ReleaseSource.
//
// It is the only code that talks to GitHub for updates, so what leaves the
// computer is decided in one place: one GET for the newest release and, when
// the release has one, one GET for its checksum file. Neither carries an
// account, a cookie, an identifier or the installed version. GitHub sees the
// computer's IP address, as every web server does.
package githubrelease

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"eincast/internal/app"
)

const (
	defaultBase = "https://api.github.com"
	// checksumsAsset is the file the release workflow attaches: "<sha256>  <name>"
	// per line, as `shasum -a 256` writes it.
	checksumsAsset = "SHA256SUMS.txt"
	// userAgent is required by GitHub. It is the same for everyone and carries no
	// version, so the request says nothing about the installed copy.
	userAgent = "EinCast"

	timeout = 15 * time.Second
	// maxBody bounds what is read back: a release description is small, and the
	// answer comes from the network.
	maxBody = 1 << 20
	// maxChecksumsBody bounds the checksum file, which is a few hundred bytes.
	maxChecksumsBody = 64 << 10
	// maxNotes bounds the description kept for display.
	maxNotes = 8000
)

// Source asks GitHub for the newest release of one repository.
type Source struct {
	repo   string // "owner/name"
	base   string
	client *http.Client
}

var _ app.ReleaseSource = (*Source)(nil)

// Option customizes a Source.
type Option func(*Source)

// WithBaseURL points the source at another API server. Tests use it.
func WithBaseURL(base string) Option {
	return func(s *Source) { s.base = strings.TrimRight(base, "/") }
}

// WithHTTPClient replaces the HTTP client. Tests use it to trust a test server.
func WithHTTPClient(c *http.Client) Option {
	return func(s *Source) { s.client = c }
}

// New returns a Source for repo, written "owner/name".
func New(repo string, opts ...Option) *Source {
	s := &Source{repo: repo, base: defaultBase, client: &http.Client{}}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// apiRelease is the part of GitHub's answer that is used.
type apiRelease struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	HTMLURL     string     `json:"html_url"`
	PublishedAt time.Time  `json:"published_at"`
	Assets      []apiAsset `json:"assets"`
}

type apiAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Latest implements app.ReleaseSource. GitHub's "latest" is the newest release
// that is neither a draft nor a pre-release.
func (s *Source) Latest(ctx context.Context) (app.Release, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := s.get(ctx, s.base+"/repos/"+s.repo+"/releases/latest", maxBody, true)
	if err != nil {
		return app.Release{}, err
	}
	var r apiRelease
	if err := json.Unmarshal(body, &r); err != nil {
		return app.Release{}, fmt.Errorf("GitHub's answer could not be read: %w", err)
	}
	if r.TagName == "" {
		return app.Release{}, fmt.Errorf("GitHub's answer has no version: %w", app.ErrNoRelease)
	}

	rel := app.Release{
		Tag: r.TagName, Name: r.Name, Notes: clip(r.Body, maxNotes),
		URL: r.HTMLURL, PublishedAt: r.PublishedAt,
	}
	sums := s.checksums(ctx, r.Assets)
	for _, a := range r.Assets {
		if a.Name == checksumsAsset {
			continue // it is how the others are verified, not something to download
		}
		rel.Assets = append(rel.Assets, app.ReleaseAsset{Name: a.Name, Size: a.Size, SHA256: sums[a.Name]})
	}
	return rel, nil
}

// checksums reads the release's checksum file. It is a convenience for the
// person verifying a download, so any trouble here (no such file, an unreachable
// server) only means the release is shown without checksums.
func (s *Source) checksums(ctx context.Context, assets []apiAsset) map[string]string {
	for _, a := range assets {
		if a.Name != checksumsAsset {
			continue
		}
		// The address comes from the network; only follow a secure one.
		if u, err := url.Parse(a.URL); err != nil || u.Scheme != "https" {
			return nil
		}
		data, err := s.get(ctx, a.URL, maxChecksumsBody, false)
		if err != nil {
			return nil
		}
		return parseChecksums(data)
	}
	return nil
}

// get fetches one address. api marks a call to the REST API, which wants its own
// Accept header; the checksum file is a plain download.
func (s *Source) get(ctx context.Context, address string, limit int64, api bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("could not build the request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if api {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, networkError(err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("GitHub has no such release: %w", app.ErrNoRelease)
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		// Unauthenticated calls are limited per IP address; a shared network can use
		// the allowance up. It resets within the hour.
		return nil, errors.New("GitHub is limiting requests from this network right now; try again in a while")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub answered with HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, networkError(err)
	}
	return data, nil
}

// networkError drops the request address that net/http puts in its errors: the
// person sees "no such host", not a URL they never typed. Context errors stay
// matchable.
func networkError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("GitHub could not be reached: %w", err)
}

// parseChecksums reads `shasum -a 256` output: "<64 hex>  <name>", where a "*"
// before the name marks binary mode. Lines that do not fit are skipped.
func parseChecksums(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		sum, name, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if !ok || name == "" || !isSHA256(sum) {
			continue
		}
		out[name] = strings.ToLower(sum)
	}
	return out
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// clip shortens s to at most n runes, marking the cut.
func clip(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
