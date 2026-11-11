package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Original-file moves in one millisecond still need separate destinations.
func TestOriginalMovesInOneMillisecond(t *testing.T) {
	sandbox(t)
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)
	was := backupNow
	backupNow = func() time.Time { return at }
	t.Cleanup(func() { backupNow = was })
	var runs []string
	for _, body := range []string{"first", "second", "third"} {
		p := filepath.Join(t.TempDir(), "config.json")
		write(t, p, body)
		dir, err := freshBackup()
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, "config.json")
		if err := os.Rename(p, dst); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, dir)
		if got := read(t, dst); got != body {
			t.Fatalf("the %s change's backup holds %q", body, got)
		}
	}
	if runs[0] == runs[1] || runs[1] == runs[2] || runs[0] == runs[2] {
		t.Fatalf("changes share a folder: %s %s %s", runs[0], runs[1], runs[2])
	}
}
