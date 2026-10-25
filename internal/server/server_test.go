package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wesleynepo/led-panel/internal/store"
	"github.com/wesleynepo/led-panel/panel"
)

func do(t *testing.T, method, url, contentType, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	New(panel.New(), store.New(t.TempDir())).Handler().ServeHTTP(rec, req)
	return rec
}

func TestRejectsForeignHostAndOrigin(t *testing.T) {
	for name, tc := range map[string]struct {
		url string
		hdr map[string]string
	}{
		"foreign host":       {"http://evil.example/api/status", nil},
		"foreign origin":     {"http://127.0.0.1:8765/api/status", map[string]string{"Origin": "http://evil.example"}},
		"ipv6 literal host":  {"http://[::1]:8765/api/status", nil},
		"local host no port": {"http://127.0.0.1/api/status", nil},
	} {
		want := 403
		if name == "local host no port" {
			want = 200
		}
		if got := do(t, "GET", tc.url, "", "", tc.hdr).Code; got != want {
			t.Errorf("%s: got %d, want %d", name, got, want)
		}
	}
}

func TestPostNeedsJSONContentType(t *testing.T) {
	const url = "http://127.0.0.1:8765/api/power"
	for ct, want := range map[string]int{
		"":                                403,
		"text/plain":                      403,
		"application/json":                400,
		"application/json; charset=utf-8": 400,
	} {
		if got := do(t, "POST", url, ct, `{"on":true}`, nil).Code; got != want {
			t.Errorf("content type %q: got %d, want %d", ct, got, want)
		}
	}
}

func TestPostBodyLimitAndSyntax(t *testing.T) {
	const url = "http://127.0.0.1:8765/api/power"
	if got := do(t, "POST", url, "application/json", `{"on":`, nil).Code; got != 400 {
		t.Errorf("bad json: got %d", got)
	}
	big := `{"text":"` + strings.Repeat("a", maxBody) + `"}`
	if got := do(t, "POST", "http://127.0.0.1:8765/api/upload-text", "application/json", big, nil).Code; got != 413 {
		t.Errorf("oversized body: got %d", got)
	}
}

func TestRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/", 200},
		{"GET", "/nope", 404},
		{"POST", "/api/status", http.StatusMethodNotAllowed},
		{"GET", "/api/connect", http.StatusMethodNotAllowed},
		{"GET", "/img/+5", 404},
		{"GET", "/img/05", 404},
		{"GET", "/img/x", 404},
	} {
		ct := ""
		if tc.method == "POST" {
			ct = "application/json"
		}
		if got := do(t, tc.method, "http://127.0.0.1:8765"+tc.path, ct, "", nil).Code; got != tc.want {
			t.Errorf("%s %s: got %d, want %d", tc.method, tc.path, got, tc.want)
		}
	}
}
