package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/profiles"
	"eincast/internal/app/resume"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
)

// Profiles is what the HTTP layer needs to know about who is watching. Declared
// here, at the consumer; profiles.Service implements it (ADR 0027).
type Profiles interface {
	// Active reports whether there is more than one profile, so a device has to
	// choose.
	Active() bool
	// Only returns the profile when there is exactly one: nobody has to choose, and
	// it is everyone's.
	Only() (profile.Profile, bool)
	Summaries() []profiles.Summary
	// Select checks the PIN (when the profile has one) and returns the credential
	// that proves the choice.
	Select(client string, id profile.ID, pin string) (string, profile.Profile, error)
	// Resolve returns the profile a credential names, if it is genuine and current.
	Resolve(credential string) (profile.Profile, bool)
	// Credential returns the credential for a profile, to put in a playlist.
	Credential(id profile.ID) (string, bool)
	// TVVisible reports whether a smart TV may see the item.
	TVVisible(it media.Item) bool
}

// Resume is what the HTTP layer needs to remember where each profile stopped.
type Resume interface {
	Get(p profile.ID) map[media.ID]app.ResumePosition
	Put(p profile.ID, id media.ID, t, d int, modTime int64) error
	Delete(p profile.ID, id media.ID)
}

const (
	// profileCookie holds the credential of the profile a browser chose. Like the
	// access cookie and the tunnel session it is its own cookie: the three are
	// never interchangeable.
	profileCookie = "eincast_profile"
	// profileParam carries the credential in playlist links, for players that
	// cannot keep a cookie.
	profileParam = "p"

	codeProfileRequired = "profile_required"
	codeWrongPIN        = "wrong_pin"

	// maxProfileBody bounds the small JSON bodies the profile and resume routes read.
	maxProfileBody = 1 << 10
)

// viewer is who a request is, as far as what it may see.
type viewer struct {
	// open means no profile rules apply (profiles are not wired in).
	open bool
	// guest means more than one profile exists and this request chose none.
	guest bool
	// profile and id are set unless open or guest.
	profile profile.Profile
	id      profile.ID
}

// viewerOf decides who a request is. With one profile everyone is it and nothing
// has to be chosen. With several, the credential names the profile, and no
// credential means guest: never "everyone", or deleting a cookie would unlock the
// library (ADR 0027).
func (a *api) viewerOf(r *http.Request) viewer {
	if a.profiles == nil {
		return viewer{open: true}
	}
	if only, ok := a.profiles.Only(); ok {
		return viewer{profile: only, id: only.ID}
	}
	for _, cred := range profileCredentials(r) {
		if p, ok := a.profiles.Resolve(cred); ok {
			return viewer{profile: p, id: p.ID}
		}
	}
	return viewer{guest: true}
}

// profileCredentials lists what the request offers as a profile credential: the
// "p" query parameter, then the cookie.
func profileCredentials(r *http.Request) []string {
	var out []string
	if v := r.URL.Query().Get(profileParam); v != "" {
		out = append(out, v)
	}
	if c, err := r.Cookie(profileCookie); err == nil && c.Value != "" {
		out = append(out, c.Value)
	}
	return out
}

// allowed is the one question every item route asks: may this request see this
// item? The answer for a forbidden item is the same 404 as for an unknown one.
func (a *api) allowed(r *http.Request, it media.Item) bool {
	v := a.viewerOf(r)
	switch {
	case v.open:
		return true
	case !v.guest:
		return v.profile.Allows(it)
	case a.hasSignedLink(r):
		// A signed link names exactly one item and was only ever issued for one the
		// profile may see.
		return true
	default:
		// A guest is how a smart TV looks, since it cannot carry a credential: it
		// gets what is marked visible to TVs, and only while Smart TVs are on.
		return a.dlna != nil && a.profiles.TVVisible(it)
	}
}

// visibleItems returns what the request may list. A guest may list nothing: the
// answer is 401 with a code the player page turns into the picker. ok is false
// when that answer has been written.
func (a *api) visibleItems(w http.ResponseWriter, r *http.Request) ([]media.Item, viewer, bool) {
	v := a.viewerOf(r)
	if v.guest {
		writeError(w, http.StatusUnauthorized, codeProfileRequired, "choose a profile")
		return nil, v, false
	}
	items := a.lib.List()
	if v.open {
		return items, v, true
	}
	return profiles.Visible(v.profile, items), v, true
}

// resumeProfile is whose positions a request reads and writes. Without profiles
// wired in there is still one person: Everyone.
func (v viewer) resumeProfile() profile.ID {
	if v.open {
		return profile.EveryoneID
	}
	return v.id
}

// ---- GET /api/profiles, POST and DELETE /api/profile ----------------------

type profileJSON struct {
	ID     profile.ID `json:"id"`
	Name   string     `json:"name"`
	Color  string     `json:"color"`
	HasPIN bool       `json:"hasPin"`
}

type profilesJSON struct {
	// Active is false with one profile: the page shows no picker.
	Active bool `json:"active"`
	// Current is the profile this browser chose, or "".
	Current  profile.ID    `json:"current"`
	Profiles []profileJSON `json:"profiles"`
}

// handleProfiles lists the profiles for the picker. It names no restrictions and
// no PINs, only who can be chosen and whether a PIN is asked.
func (a *api) handleProfiles(w http.ResponseWriter, r *http.Request) {
	out := profilesJSON{Profiles: []profileJSON{}}
	if a.profiles != nil && a.profiles.Active() {
		out.Active = true
		if v := a.viewerOf(r); !v.guest && !v.open {
			out.Current = v.id
		}
		for _, s := range a.profiles.Summaries() {
			out.Profiles = append(out.Profiles, profileJSON{ID: s.ID, Name: s.Name, Color: s.Color, HasPIN: s.HasPIN})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

type chooseBody struct {
	ID  profile.ID `json:"id"`
	PIN string     `json:"pin"`
}

// handleChooseProfile checks the PIN and, if right, sets the profile cookie.
func (a *api) handleChooseProfile(w http.ResponseWriter, r *http.Request) {
	if a.profiles == nil || !a.profiles.Active() {
		a.handleNotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if !a.sameSite(r) {
		writeError(w, http.StatusForbidden, codeForbidden, "cross-site request refused")
		return
	}
	var body chooseBody
	if !a.readJSON(w, r, &body) {
		return
	}

	ip := clientIP(r)
	cred, p, err := a.profiles.Select(ip, body.ID, body.PIN)
	var locked *app.LockedError
	switch {
	case err == nil:
		http.SetCookie(w, &http.Cookie{
			Name:     profileCookie,
			Value:    cred,
			Path:     "/",
			MaxAge:   int(cookieMaxAge / time.Second),
			HttpOnly: true,
			Secure:   requestScheme(r) == "https",
			SameSite: http.SameSiteLaxMode,
		})
		a.log.InfoContext(r.Context(), "profile chosen", slog.String("profile", p.Name), slog.String("client_ip", ip))
		writeJSON(w, http.StatusOK, profileJSON{ID: p.ID, Name: p.Name, Color: p.Color})
	case errors.As(err, &locked):
		a.log.WarnContext(r.Context(), "profile PIN locked out", slog.String("client_ip", ip), slog.Duration("retry_after", locked.RetryAfter))
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(locked.RetryAfter.Seconds()))))
		writeError(w, http.StatusTooManyRequests, "locked", "too many wrong PINs; try again in a few minutes")
	case errors.Is(err, profiles.ErrWrongPIN):
		a.log.WarnContext(r.Context(), "wrong profile PIN", slog.String("client_ip", ip))
		writeError(w, http.StatusUnauthorized, codeWrongPIN, "that PIN didn't work")
	case errors.Is(err, profiles.ErrUnknownProfile):
		writeError(w, http.StatusNotFound, codeNotFound, "no such profile")
	default:
		a.log.ErrorContext(r.Context(), "choosing a profile failed", errAttr(err))
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
	}
}

// handleLeaveProfile forgets the choice, so the picker comes back.
func (a *api) handleLeaveProfile(w http.ResponseWriter, r *http.Request) {
	if !a.sameSite(r) {
		writeError(w, http.StatusForbidden, codeForbidden, "cross-site request refused")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: profileCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

// readJSON reads a small JSON body, answering 400 itself when it cannot.
func (a *api) readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read the request")
		return false
	}
	return true
}

// ---- /api/resume ------------------------------------------------------------

type resumeJSON struct {
	ID media.ID `json:"id"`
	T  int      `json:"t"`
	D  int      `json:"d"`
	M  int64    `json:"m"`
	S  int64    `json:"s"`
}

// handleResumeList returns where this profile stopped, most recent first, for the
// titles it may still see. Positions of titles that are gone or not allowed are
// not returned, so the list never names something the profile cannot open.
func (a *api) handleResumeList(w http.ResponseWriter, r *http.Request) {
	v := a.viewerOf(r)
	if v.guest {
		writeError(w, http.StatusUnauthorized, codeProfileRequired, "choose a profile")
		return
	}
	saved := a.resume.Get(v.resumeProfile())
	out := make([]resumeJSON, 0, len(saved))
	for _, id := range resume.Newest(saved) {
		it, err := a.lib.Get(id)
		if err != nil || !a.allowed(r, it) {
			continue
		}
		pos := saved[id]
		out = append(out, resumeJSON{ID: id, T: pos.T, D: pos.D, M: pos.M, S: pos.S})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

type putResumeBody struct {
	T int   `json:"t"`
	D int   `json:"d"`
	M int64 `json:"m"`
}

// handleResumePut records a position for a title the profile may see.
func (a *api) handleResumePut(w http.ResponseWriter, r *http.Request) {
	it, v, ok := a.resumeTarget(w, r)
	if !ok {
		return
	}
	var body putResumeBody
	if !a.readJSON(w, r, &body) {
		return
	}
	if err := a.resume.Put(v.resumeProfile(), it.ID, body.T, body.D, body.M); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "that is not a position in this title")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleResumeDelete forgets a position, for a title that was finished.
func (a *api) handleResumeDelete(w http.ResponseWriter, r *http.Request) {
	it, v, ok := a.resumeTarget(w, r)
	if !ok {
		return
	}
	a.resume.Delete(v.resumeProfile(), it.ID)
	w.WriteHeader(http.StatusNoContent)
}

// resumeTarget checks the request may touch this title's position: not a guest,
// not a cross-site request, and the title one the profile may see.
func (a *api) resumeTarget(w http.ResponseWriter, r *http.Request) (media.Item, viewer, bool) {
	v := a.viewerOf(r)
	if v.guest {
		writeError(w, http.StatusUnauthorized, codeProfileRequired, "choose a profile")
		return media.Item{}, v, false
	}
	if !a.sameSite(r) {
		writeError(w, http.StatusForbidden, codeForbidden, "cross-site request refused")
		return media.Item{}, v, false
	}
	it, ok := a.lookup(w, r)
	return it, v, ok
}

// tvLibrary is the library as a smart TV may see it: only what is marked visible
// to TVs. A TV cannot say who is watching, so it gets one rule for everyone.
type tvLibrary struct {
	lib Library
	tv  interface{ TVVisible(media.Item) bool }
}

func (t tvLibrary) List() []media.Item {
	all := t.lib.List()
	out := make([]media.Item, 0, len(all))
	for _, it := range all {
		if t.tv.TVVisible(it) {
			out = append(out, it)
		}
	}
	return out
}
