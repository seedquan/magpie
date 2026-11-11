package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/agent"
)

func seedLegacyBiliState(t *testing.T, state map[string][]biliFile) string {
	t.Helper()
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, biliStateFile(), string(b))
	return string(b)
}

func TestBiliNativeInstallCreatesNoRecoveryCopies(t *testing.T) {
	for _, id := range []string{"pi", "omp", "opencode", "claude"} {
		t.Run(id, func(t *testing.T) {
			h := sandbox(t)
			write(t, filepath.Join(h, ".omp/agent/config.yml"), "theme: dark\n")
			holdCLI(t, h, "omp")
			f := newFakeBili(t, h)
			a, err := agent.Find(id)
			if err != nil {
				t.Fatal(err)
			}
			paths := biliLanes[id](a)
			state := "{\n  \"legacy\": []\n}\n"
			write(t, biliStateFile(), state)
			originals := map[string]string{}
			for _, p := range paths {
				originals[p] = read(t, p)
				write(t, p+".bili-bak", "legacy backup")
				write(t, p+".bili-last", "legacy last")
			}
			if _, err := SetBili(id, true); err == nil || !strings.Contains(err.Error(), "recovery") {
				t.Errorf("native install must explain the recovery-copy restriction: %v", err)
			}
			if len(filter(f.runs, "plugin install", "plugin remove")) != 0 {
				t.Errorf("ran external native installer/remover: %v", f.runs)
			}
			if got := read(t, biliStateFile()); got != state {
				t.Error("legacy install state changed")
			}
			for p, original := range originals {
				if got := read(t, p); got != original {
					t.Errorf("native install changed %s", p)
				}
				if read(t, p+".bili-bak") != "legacy backup" || read(t, p+".bili-last") != "legacy last" {
					t.Errorf("legacy sidecars changed for %s", p)
				}
			}
		})
	}
}

func TestBiliLegacyUndoReadsEveryFileBeforeWriting(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("requires Unix file permissions")
	}
	h := sandbox(t)
	f := newFakeBili(t, h)
	cc := filepath.Join(h, ".claude/settings.json")
	command := filepath.Join(h, ".claude/commands/acp-cache.md")
	before, after, commandAfter := `{"model":"mine"}`, `{"model":"mine","hooks":{"SessionStart":[]}}`, "bili command\n"
	write(t, cc, after)
	write(t, command, commandAfter)
	state := seedLegacyBiliState(t, map[string][]biliFile{"claude": {
		{Path: cc, Before: &before, After: &after},
		{Path: command, After: &commandAfter},
	}})
	if err := os.Chmod(command, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(command, 0o644) })
	if _, err := SetBili("claude", false); err == nil {
		t.Fatal("undo accepted an unreadable command file")
	}
	if read(t, cc) != after || read(t, biliStateFile()) != state {
		t.Fatal("undo changed an earlier file or the legacy state before a later read failed")
	}
	if len(filter(f.runs, "plugin install", "plugin remove")) != 0 {
		t.Fatalf("ran external native installer/remover: %v", f.runs)
	}
	if err := os.Chmod(command, 0o644); err != nil {
		t.Fatal(err)
	}
	if read(t, command) != commandAfter {
		t.Fatal("unreadable original was changed")
	}
}

func TestBiliLegacyUndoKeepsDamagedFilesAndState(t *testing.T) {
	for _, damage := range []string{"configuration", "state", "incomplete record"} {
		t.Run(damage, func(t *testing.T) {
			h := sandbox(t)
			f := newFakeBili(t, h)
			p := filepath.Join(h, ".pi/agent/settings.json")
			before, after := `{"theme":"dark"}`, `{"theme":"dark","packages":["npm:billion-context"]}`
			write(t, p, after)
			state := seedLegacyBiliState(t, map[string][]biliFile{"pi": {{Path: p, Before: &before, After: &after}}})
			if damage == "configuration" {
				write(t, p, `{"theme":"dark","packages":`)
			} else if damage == "state" {
				state = `{"pi":[{"path":`
				write(t, biliStateFile(), state)
			} else {
				state = seedLegacyBiliState(t, map[string][]biliFile{"pi": {{Path: p}}})
			}
			original := read(t, p)
			if _, err := SetBili("pi", false); err == nil {
				t.Error("undo accepted damaged configuration or legacy state")
			}
			if read(t, p) != original || read(t, biliStateFile()) != state {
				t.Fatal("damaged original or legacy state changed")
			}
			if len(filter(f.runs, "plugin install", "plugin remove")) != 0 {
				t.Fatalf("ran external native installer/remover: %v", f.runs)
			}
		})
	}
}

func TestBiliLegacyUndoRejectsChangedInstalledValues(t *testing.T) {
	for _, tt := range []struct {
		name, id, before, after, current string
	}{
		{
			name: "Pi packages extended", id: "pi",
			before:  `{"theme":"dark"}`,
			after:   `{"theme":"dark","packages":["npm:billion-context"]}`,
			current: `{"theme":"light","packages":["npm:billion-context","npm:extra"]}`,
		},
		{
			name: "Claude hooks extended", id: "claude",
			before:  `{"model":"mine","env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:3425"}}`,
			after:   `{"model":"mine","env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:18787/bili/http://127.0.0.1:3425"},"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"node claude-native-bootstrap.js"}]}]}}`,
			current: `{"model":"new","env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:18787/bili/http://127.0.0.1:3425"},"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"node claude-native-bootstrap.js"}]},{"hooks":[{"type":"command","command":"echo mine"}]}]}}`,
		},
		{
			name: "OpenCode hooks extended", id: "opencode",
			before:  `{"theme":"dark","hooks":{"session.created":[]}}`,
			after:   `{"theme":"dark","hooks":{"session.created":[{"command":"bili hook session-start"}]}}`,
			current: `{"theme":"light","hooks":{"session.created":[{"command":"bili hook session-start"},{"command":"echo mine"}]}}`,
		},
		{
			name: "unpatchable installed key", id: "opencode",
			before:  `{"theme":"dark"}`,
			after:   `{"theme":"dark","bili@hook":"bili hook session-start"}`,
			current: `{"theme":"light","bili@hook":"bili hook session-start"}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := sandbox(t)
			f := newFakeBili(t, h)
			a, err := agent.Find(tt.id)
			if err != nil {
				t.Fatal(err)
			}
			paths := biliLanes[tt.id](a)
			originals := map[string]string{}
			var snap []biliFile
			for _, p := range paths {
				before, after, current := tt.before, tt.after, tt.current
				if p != a.Path {
					before, after, current = "old command\n", "bili command\n", "bili command\n"
				}
				marks := []string{p + ".bili-bak", p + ".bili-last"}
				snap = append(snap, biliFile{Path: p, Before: &before, After: &after, Marks: marks})
				write(t, p, current)
				originals[p] = current
				for _, mark := range marks {
					write(t, mark, "legacy sidecar\n")
					originals[mark] = "legacy sidecar\n"
				}
			}
			originals[biliStateFile()] = seedLegacyBiliState(t, map[string][]biliFile{tt.id: snap})
			if v, err := SetBili(tt.id, false); err == nil || !strings.Contains(err.Error(), "repair") || v != nil {
				t.Fatalf("undo must require repair instead of reporting success with installed values remaining: %v", err)
			}
			for p, original := range originals {
				if read(t, p) != original {
					t.Errorf("unsafe undo changed %s", p)
				}
			}
			if len(filter(f.runs, "plugin install", "plugin remove")) != 0 {
				t.Fatal("unsafe undo ran an external installer/remover")
			}
		})
	}
}

func TestLinkedInstructionsUnreadableEditPreservesOriginal(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("requires Unix file permissions")
	}
	h := sandbox(t)
	p := filepath.Join(h, "shared/AGENTS.md")
	const original = "User instructions.\n"
	write(t, p, original)
	ok(t)(SaveInstructions(InstructionsChange{Create: &InstrSet{ID: "linked", Name: "Linked", File: p}}))
	legacy := filepath.Join(BackupDir(), "legacy/library/AGENTS.md")
	write(t, legacy, "legacy instructions")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	replacement := "New instructions."
	if _, err := SaveInstructions(InstructionsChange{Texts: map[string]*string{"linked": &replacement}}); err == nil {
		t.Error("an unreadable linked file was overwritten")
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if read(t, p) != original || read(t, legacy) != "legacy instructions" {
		t.Fatal("original or legacy snapshot changed")
	}
	entries, err := os.ReadDir(BackupDir())
	if err != nil || len(entries) != 1 || !slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == "legacy" }) {
		t.Fatalf("snapshots created or removed: %v, %v", entries, err)
	}
}
