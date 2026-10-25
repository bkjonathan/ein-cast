package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

func TestRequestID(t *testing.T) {
	tests := []struct {
		name    string
		inbound string
		keep    bool
	}{
		{"none supplied", "", false},
		{"plain id kept", "abc-123_XYZ", true},
		{"spaces rejected", "has space", false},
		{"too long rejected", strings.Repeat("a", 65), false},
		{"punctuation rejected", "a;b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = requestIDFrom(r.Context())
			}))
			req := httptest.NewRequest("GET", "/", nil)
			if tt.inbound != "" {
				req.Header.Set("X-Request-ID", tt.inbound)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Request-ID")
			if got != seen || got == "" {
				t.Fatalf("header %q vs context %q", got, seen)
			}
			if tt.keep && got != tt.inbound {
				t.Errorf("id = %q, want inbound %q kept", got, tt.inbound)
			}
			if !tt.keep && len(got) != 16 {
				t.Errorf("id = %q, want a generated 16-char id", got)
			}
		})
	}
}

func TestRecoverer(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	panics := true
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if panics {
			panic("kaboom")
		}
		w.WriteHeader(http.StatusNoContent)
	}), requestID, accessLog(log), recoverer(log))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	assertRecorderError(t, rec, 500, "internal")
	if !strings.Contains(logs.String(), "kaboom") || !strings.Contains(logs.String(), `"status":500`) {
		t.Errorf("log should record the panic and the 500:\n%s", logs.String())
	}

	// The server keeps serving after a panic.
	panics = false
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("request after a panic: status = %d", rec.Code)
	}
}

func assertRecorderError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not the error shape: %v", rec.Body.String(), err)
	}
	if rec.Code != status || body.Error.Code != code {
		t.Fatalf("status=%d code=%q, want %d %q", rec.Code, body.Error.Code, status, code)
	}
}

func TestRecoverer_PropagatesErrAbortHandler(t *testing.T) {
	h := recoverer(discardLog())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if r := recover(); r != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler to keep propagating", r)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestCORSLAN(t *testing.T) {
	tests := []struct {
		origin  string
		allowed bool
	}{
		{"http://localhost:5173", true},
		{"http://127.0.0.1:8765", true},
		{"http://[::1]:3000", true},
		{"http://192.168.1.20", true},
		{"http://10.0.0.5:8080", true},
		{"http://172.16.4.2", true},
		{"http://169.254.1.1", true},
		{"http://eincast.local", true},
		{"https://MacBook.LOCAL", true},
		{"https://evil.example.com", false},
		{"http://8.8.8.8", false},
		{"http://192.168.1.20.evil.com", false},
		{"http://localhost.evil.com", false},
		{"null", false},
		{"file://", false},
		{"ftp://192.168.1.1", false},
	}
	h := corsLAN(http.HandlerFunc(ok))
	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/library", nil)
			req.Header.Set("Origin", tt.origin)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get("Access-Control-Allow-Origin")
			if tt.allowed && got != tt.origin {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.origin)
			}
			if !tt.allowed && got != "" {
				t.Errorf("Allow-Origin = %q, want none for a non-LAN origin", got)
			}
			if rec.Header().Get("Vary") != "Origin" {
				t.Errorf("Vary = %q, want Origin", rec.Header().Get("Vary"))
			}
		})
	}
}

func TestCORSLAN_NoOriginMeansNoHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	corsLAN(http.HandlerFunc(ok)).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Vary") != "" {
		t.Errorf("headers = %v", rec.Header())
	}
}

func TestCORSLAN_Preflight(t *testing.T) {
	called := false
	h := corsLAN(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest("OPTIONS", "/stream/x", nil)
	req.Header.Set("Origin", "http://192.168.1.9")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent || called {
		t.Fatalf("status=%d handlerCalled=%v, want 204 answered by the middleware", rec.Code, called)
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Range") {
		t.Errorf("preflight must allow the Range header: %v", rec.Header())
	}

	// A preflight from a public origin gets nothing.
	req.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("public origin received CORS headers")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct{ remote, xff, want string }{
		{"192.168.1.9:51234", "", "192.168.1.9"},
		{"[fe80::1]:80", "", "fe80::1"},
		{"192.168.1.9:51234", "1.2.3.4", "192.168.1.9"}, // forwarding headers are not trusted
		{"garbage", "", "garbage"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = tt.remote
		if tt.xff != "" {
			req.Header.Set("X-Forwarded-For", tt.xff)
		}
		if got := clientIP(req); got != tt.want {
			t.Errorf("clientIP(%q, xff=%q) = %q, want %q", tt.remote, tt.xff, got, tt.want)
		}
	}
}

func TestContentType(t *testing.T) {
	tests := map[string]string{
		"a.mkv":  "video/x-matroska",
		"A.MKV":  "video/x-matroska",
		"a.mp4":  "video/mp4",
		"a.ts":   "video/mp2t",
		"a.flac": "audio/flac",
		"a.mp3":  "audio/mpeg",
		"a.heic": "image/heic",
		"a.png":  "image/png",
		"a.zzzz": "application/octet-stream",
		"noext":  "application/octet-stream",
	}
	for name, want := range tests {
		if got := contentType(name); got != want {
			t.Errorf("contentType(%q) = %q, want %q", name, got, want)
		}
	}
}
