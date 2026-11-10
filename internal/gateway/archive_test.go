package gateway

import (
	"context"
	"github.com/yetone/magpie/internal/settings"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// memBucket is a bucket in memory, or one that refuses every write.
type memBucket struct {
	mu   sync.Mutex
	objs map[string][]byte
	err  error
}

func (b *memBucket) Put(_ context.Context, name string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	b.objs[name] = data
	return nil
}

func archiveTo(t *testing.T, b *memBucket) {
	t.Helper()
	was := ArchiveBucket
	ArchiveBucket = func() (Putter, bool) { return b, true }
	t.Cleanup(func() { ArchiveBucket = was })
}

// archiveVendor answers a chat request, with a cookie of its own, and
// keeps nothing of what it was sent.
type archiveVendor struct{}

func (archiveVendor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Set-Cookie", "vendor_session=abc123; HttpOnly")
	io.WriteString(w, `{"id":"c1","object":"chat.completion","model":"m1","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
}

const archiveKey = "sk-proj-abcdefghijklmnopqrstuvwxyz0123456789"

func archivePost(t *testing.T, s *Server) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":"fake/m1","messages":[{"role":"user","content":"deploy with ` + archiveKey + `"}],"api_key":"named-secret"}`
	req := httptest.NewRequest("POST", "/v1/chat/completions?key=query-secret&beta=true", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer magpie-gateway-key")
	req.Header.Set("x-api-key", "another-secret")
	req.Header.Set("Cookie", "sid=cookie-secret")
	req.Header.Set("User-Agent", "claude-cli/2.1.0")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	return rec
}

func TestLegacyArchiveSettingsCannotExport(t *testing.T) {
	fresh(t)
	serveOn(t, "fake", "k", []string{"m1"}, archiveVendor{})
	if err := settings.Save(settings.Settings{RequestArchive: true, RequestArchiveLocal: true}); err != nil {
		t.Fatal(err)
	}
	b := &memBucket{objs: map[string][]byte{}}
	archiveTo(t, b)
	called := false
	ArchiveBucket = func() (Putter, bool) { called = true; return b, true }
	s := New()
	rec := archivePost(t, s)
	if called || len(b.objs) != 0 || rec.Header().Get(ArchiveHeader) != "" || s.Recent()[0].Archive != "" {
		t.Fatal("legacy settings enabled request export")
	}
	if _, err := os.Stat(filepath.Join(settings.Dir(), "request-archive")); !os.IsNotExist(err) {
		t.Fatalf("local archive was created: %v", err)
	}
}
