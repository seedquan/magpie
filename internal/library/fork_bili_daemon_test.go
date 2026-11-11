package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/settings"
)

func TestForkBiliUsesOnlyAnExistingDaemon(t *testing.T) {
	h := sandbox(t)
	old := biliServing
	biliServing = func() bool { return false }
	t.Cleanup(func() { biliServing = old })
	if err := biliServe(filepath.Join(h, "missing-bili")); err == nil || !strings.Contains(err.Error(), "start it yourself") {
		t.Fatalf("missing daemon should require an explicit external start: %v", err)
	}
	if _, err := os.Stat(filepath.Join(settings.Dir(), "bili-start.log")); !os.IsNotExist(err) {
		t.Fatalf("magpie tried to start a daemon: %v", err)
	}
	biliServing = func() bool { return true }
	if err := biliServe(filepath.Join(h, "missing-bili")); err != nil {
		t.Fatalf("existing daemon was not reused: %v", err)
	}
}

func TestForkBiliNeverRestartsADaemonInBackground(t *testing.T) {
	h := sandbox(t)
	newFakeBili(t, h)
	dir := filepath.Join(h, "zed-config")
	t.Setenv("MAGPIE_ZED_CONFIG_DIR", dir)
	write(t, filepath.Join(dir, "settings.json"), `{"language_models":{"openai_compatible":{"magpie":{"api_url":"`+biliPrefix+gateway.URL()+`/v1"}}}}`)
	oldServe, oldServing := biliServe, biliServing
	calls := 0
	biliServe = func(string) error { calls++; return nil }
	biliServing = func() bool { return false }
	t.Cleanup(func() { biliServe, biliServing = oldServe, oldServing })
	keepBiliServe(context.Background())
	if calls != 0 {
		t.Fatal("background work tried to start an external daemon")
	}
}
