package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// plugin-updates.json is read as no updates when it doesn't parse, and the
// next check wrote that over it: Updated, the plugins magpie updated by
// itself, can't be found again (#2263). What was there is copied aside
// first.
func TestUnreadUpdatesLeftIntact(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	if err := os.MkdirAll(filepath.Dir(updatesPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	was := `{"updated":[{"package":"opencode-gemini-auth","from":"0.1.2","to":"0.2`
	if err := os.WriteFile(updatesPath(), []byte(was), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckUpdates(context.Background()); err != nil {
		t.Fatal(err)
	}
	bad, _ := filepath.Glob(updatesPath() + ".bad-*")
	if len(bad) != 0 {
		t.Fatalf("created recovery copies: %v", bad)
	}
	if b, _ := os.ReadFile(updatesPath()); string(b) != was {
		t.Fatal("damaged update history overwritten")
	}
	if !PendingUpdates().Checked.IsZero() {
		t.Fatal("damaged update history rewritten")
	}
}
