package gui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

type partnerFeedTransport func(*http.Request) (*http.Response, error)

func (f partnerFeedTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Go through the browser's real handler and origin guard: reads and rejected
// writes never fetch; one explicit POST fetches the official feed without
// forwarding the browser's credentials. The feed is entirely local here.
func TestPartnersRefreshRequiresExplicitOwnPagePost(t *testing.T) {
	sandboxHome(t)
	t.Setenv("MAGPIE_PARTNERS", "")
	var requests atomic.Int32
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: partnerFeedTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != provider.PartnersURL {
			return old.Do(r)
		}
		requests.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("partner fetch forwarded credentials or used a non-GET method")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"partners":[{"id":"manual-gui","name":"Manual GUI","chat":"https://manual.example/v1"}]}`)), Request: r}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = old })
	h := webOwnPage(Handler(nil, nil))
	call := func(method, path, site string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://magpie.test"+path, strings.NewReader(`{}`))
		r.Header.Set("Sec-Fetch-Site", site)
		r.RemoteAddr = "127.0.0.1:50000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/", "/api/providers", "/api/partners/refresh"} {
		call(http.MethodGet, path, "same-origin")
	}
	if got := call(http.MethodPost, "/api/partners/refresh", "cross-site"); got.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST: %d", got.Code)
	}
	if requests.Load() != 0 {
		t.Fatal("page load, provider read or rejected POST fetched partners")
	}
	got := call(http.MethodPost, "/api/partners/refresh", "same-origin")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"id":"manual-gui"`) || requests.Load() != 1 {
		t.Fatalf("manual POST: status %d, partner requests %d", got.Code, requests.Load())
	}
	provider.PartnersNow(time.Second)
	call(http.MethodGet, "/api/providers", "same-origin")
	if requests.Load() != 1 {
		t.Fatal("subsequent list reads fetched partners again")
	}
}
