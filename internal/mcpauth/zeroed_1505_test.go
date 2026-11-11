package mcpauth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// mcp-signins.json zeroed by a crash (#1505) can read an existing legacy
// backup; with none, a sign-out doesn't overwrite sign-ins that don't read.
func TestZeroedMCPSignInsReadFromBackup(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	for _, n := range []string{"a", "b"} {
		if err := update(func(m map[string]*Record) bool { m[n] = &Record{URL: "https://" + n + ".invalid/mcp"}; return true }); err != nil {
			t.Fatal(err)
		}
		if n == "a" {
			legacy, err := os.ReadFile(path())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path()+".bak", legacy, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	zeroed := make([]byte, 900)
	if err := os.WriteFile(path(), zeroed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Get("a"); !ok {
		t.Fatal("a zeroed mcp-signins.json read as no sign-in to a")
	}

	if err := os.Remove(path() + ".bak"); err != nil {
		t.Fatal(err)
	}
	if err := SignOut("b"); err == nil {
		t.Fatal("a sign-out over sign-ins that don't read said done")
	}
	if got, _ := os.ReadFile(path()); !bytes.Equal(got, zeroed) {
		t.Fatalf("a sign-out wrote %q over sign-ins that don't read", got)
	}
}
