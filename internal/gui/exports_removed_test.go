package gui

import (
	"github.com/yetone/magpie/internal/settings"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemovedDataExportRoutes(t *testing.T) {
	sandboxHome(t)
	if err := settings.Save(settings.Settings{RequestArchive: true, RequestArchiveLocal: true}); err != nil {
		t.Fatal(err)
	}
	h := Handler(folderOnly{}, nil)
	for _, path := range []string{
		"/api/backup", "/api/backup/export", "/api/backup/restore", "/api/backup/import",
		"/api/autobackup", "/api/autobackup/now", "/api/autobackup/set",
		"/api/davsync", "/api/davsync/set", "/api/davsync/now",
		"/api/archive", "/api/archive/file", "/api/archive/export", "/api/settings/archive",
		"/api/sessions/markdown?agent=codex&id=missing", "/api/sessions/export",
		"/api/usage/requests.csv", "/api/usage/requests/export",
	} {
		for _, method := range []string{"GET", "POST"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader("{}")))
			if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
				t.Errorf("removed export %s %s returned %d", method, path, w.Code)
			}
		}
	}
}
