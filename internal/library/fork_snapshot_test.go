package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLibraryConfigChangesCreateNoSnapshotsAndRetainOldOnes(t *testing.T) {
	h := sandbox(t)
	const oldCount = 35
	for i := range oldCount {
		write(t, filepath.Join(BackupDir(), fmt.Sprintf("2025-01-%02d", i+1), "claude", "CLAUDE.md"), fmt.Sprintf("old %d", i))
	}
	target := filepath.Join(h, ".claude", "CLAUDE.md")
	write(t, target, "User instructions\n")
	shared := "Shared instructions"
	res, err := SaveInstructions(InstructionsChange{Shared: &shared, Agents: []string{"claude"}})
	if err != nil || len(res.Problems) != 0 {
		t.Fatalf("save instructions: %+v, %v", res, err)
	}
	if !strings.Contains(read(t, target), shared) {
		t.Fatal("configuration edit did not reach the agent")
	}
	pruneBackups()
	entries, err := os.ReadDir(BackupDir())
	if err != nil || len(entries) != oldCount || res.Backup != "" {
		t.Fatalf("snapshots created or removed: count=%d, result=%+v, err=%v", len(entries), res, err)
	}
	for i := range oldCount {
		if got := read(t, filepath.Join(BackupDir(), fmt.Sprintf("2025-01-%02d", i+1), "claude", "CLAUDE.md")); got != fmt.Sprintf("old %d", i) {
			t.Fatalf("legacy snapshot %d changed: %q", i, got)
		}
	}
}

func TestLibrarySkillEditCreatesNoSnapshot(t *testing.T) {
	h := sandbox(t)
	src := filepath.Join(h, "source-skills")
	skill(t, filepath.Join(src, "pdf"), "pdf", "Original")
	ok(t)(InstallSkills(src, []string{"pdf"}, []string{"codex"}))
	ok(t)(SetSkillHow("", HowCopy))
	copy := filepath.Join(h, ".codex/skills/pdf")
	when := time.Now().Add(time.Hour)
	write(t, filepath.Join(copy, "SKILL.md"), "---\nname: pdf\ndescription: Agent edit\n---\n")
	if err := os.Chtimes(filepath.Join(copy, "SKILL.md"), when, when); err != nil {
		t.Fatal(err)
	}
	ok(t)(Sync())
	if got := read(t, filepath.Join(src, "pdf/SKILL.md")); !strings.Contains(got, "Agent edit") {
		t.Fatalf("skill edit did not reach the library: %q", got)
	}
	if entries, err := os.ReadDir(BackupDir()); err == nil && len(entries) != 0 || err != nil && !os.IsNotExist(err) {
		t.Fatalf("skill snapshot created: %v, %v", entries, err)
	}
	// Unique originals are still moved, so taking an agent's skill out of
	// the way never deletes the sole remaining version.
	own := filepath.Join(h, "unique-skill")
	write(t, filepath.Join(own, "SKILL.md"), "unique original")
	moved, err := setAside("codex", own)
	if err != nil || read(t, filepath.Join(moved, "SKILL.md")) != "unique original" {
		t.Fatalf("unique original not preserved: %q, %v", moved, err)
	}
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatalf("set-aside copied instead of moving: %v", err)
	}
	pruneBackups()
	if got := read(t, filepath.Join(moved, "SKILL.md")); got != "unique original" {
		t.Fatal("unique original was pruned")
	}
}
