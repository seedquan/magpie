package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestForkPartnersNeverFetch(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	resetPartners()
	t.Cleanup(resetPartners)
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"partners":[]}`))
	}))
	defer srv.Close()
	t.Setenv("MAGPIE_PARTNERS", srv.URL)
	path := partnerCacheFile()
	os.MkdirAll(filepath.Dir(path), 0o700)
	cached := []byte(`{"partners":[{"id":"cached","name":"Cached","chat":"https://cached.example/v1","iconUrl":"` + srv.URL + `/icon"}],"seen":{"old":{"id":"old","name":"Old","chat":"https://old.example/v1"}}}`)
	// The cache validator rejects HTTP icon URLs; it must never try to fetch them.
	os.WriteFile(path, cached, 0o600)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	KeepPartnersFresh(ctx)
	Partners()
	PartnersNow(time.Second)
	if requests.Load() != 0 {
		t.Fatalf("partner background/lazy paths made %d requests", requests.Load())
	}
	if got, _ := os.ReadFile(path); string(got) != string(cached) {
		t.Fatal("a read rewrote the cached partner list")
	}
	if p := partnerPreset("old"); p == nil || p.Chat != "https://old.example/v1" {
		t.Fatal("an existing provider lost its cached preset")
	}
}

func TestForkPartnersManualFetch(t *testing.T) {
	cachedPartners(t)
	var requests atomic.Int32
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("feed request carried credentials or unexpected method")
		}
		if fail.Load() {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"partners":[{"id":"manual","name":"Manual","chat":"https://manual.example/v1","iconUrl":"https://manual.example/icon"}]}`))
	}))
	defer srv.Close()
	oldURL, oldIcon := partnerFeedURL, fetchPartnerIcon
	partnerFeedURL = srv.URL
	icons := 0
	fetchPartnerIcon = func(context.Context, string) (string, error) { icons++; return "generic", nil }
	t.Cleanup(func() { partnerFeedURL, fetchPartnerIcon = oldURL, oldIcon })
	if err := RefreshPartners(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || icons != 1 || partnerIDs(Partners()) != "manual" {
		t.Fatal("explicit fetch did not update list and logos")
	}
	if p := partnerPreset("old"); p == nil {
		t.Fatal("manual refresh lost a past provider preset")
	}
	resetPartners()
	if partnerIDs(PartnersNow(time.Second)) != "manual" || requests.Load() != 1 {
		t.Fatal("restart/lazy read refetched")
	}
	before, _ := os.ReadFile(partnerCacheFile())
	fail.Store(true)
	if err := RefreshPartners(context.Background()); err == nil {
		t.Fatal("failed fetch said done")
	}
	after, _ := os.ReadFile(partnerCacheFile())
	if string(before) != string(after) || partnerIDs(Partners()) != "manual" {
		t.Fatal("failed fetch changed prior list")
	}
	if requests.Load() != 2 {
		t.Fatal("manual failure retried automatically")
	}
}
