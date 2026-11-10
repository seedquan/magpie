package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/sessions"
)

// Empty sessions remain viewable even though Markdown file export is removed.
func TestEmptySessionOpens(t *testing.T) {
	h := sandboxHome(t)
	codex := filepath.Join(h, ".codex")
	t.Setenv("CODEX_HOME", codex)
	const id = "01a12261-d5bb-7f03-a40e-c4adca978a90"
	b, err := os.ReadFile(filepath.Join("..", "sessions", "testdata", "codex-aborted-helper.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(codex, "sessions", "2026", "10", "10")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-10-10T04-36-39-"+id+".jsonl"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	sessions.Reset()
	t.Cleanup(sessions.Reset)

	mux := http.NewServeMux()
	sessionRoutes(mux, folderOnly{})
	sessionManageRoutes(mux, folderOnly{})
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	if w := get("/api/sessions/manage?agent=codex"); !strings.Contains(w.Body.String(), id) {
		t.Fatalf("not listed: %s", w.Body)
	}
	if w := get("/api/sessions/transcript?agent=codex&id=" + id); w.Code != 200 || strings.Contains(w.Body.String(), "no such session") {
		t.Fatalf("conversation: %d %s", w.Code, w.Body)
	}
	if w := get("/api/sessions/markdown?agent=codex&id=" + id); w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("removed Markdown export remains reachable: %d %s", w.Code, w.Body)
	}
}
