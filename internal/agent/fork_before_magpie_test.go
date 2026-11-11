package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBeforeMagpiePreservesLegacyCopiesWithoutCreatingMore(t *testing.T) {
	home := syncHome(t)
	first, second := filepath.Join(home, "settings.json"), filepath.Join(home, "models.json")
	writeFile(t, first, `{"theme":"current"}`)
	writeFile(t, first+".before-magpie", "{ \"theme\": \"older\" }\n")
	writeFile(t, second+".before-magpie", "legacy model data")
	b := beforeMagpie{"fork.before.absent", []string{first, second}}
	b.take()
	if got, _ := os.ReadFile(first + ".before-magpie"); string(got) != "{ \"theme\": \"older\" }\n" {
		t.Fatalf("legacy settings copy overwritten: %q", got)
	}
	if got, _ := os.ReadFile(second + ".before-magpie"); string(got) != "legacy model data" {
		t.Fatalf("legacy copy deleted for an absent file: %q", got)
	}
	if err := b.restore(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(first); string(got) != `{"theme":"current"}` {
		t.Fatalf("stale snapshot restored over current values: %q", got)
	}
	// Compatibility still reads a matching legacy snapshot for its layout.
	writeFile(t, first, `{"theme":"older"}`)
	if err := b.restore(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(first); string(got) != "{ \"theme\": \"older\" }\n" {
		t.Fatalf("legacy formatting was not restored: %q", got)
	}
	for _, f := range []string{first, second} {
		if _, err := os.Stat(f + ".before-magpie"); err != nil {
			t.Fatalf("legacy recovery copy removed: %v", err)
		}
	}
	newFile := filepath.Join(home, "fresh.json")
	writeFile(t, newFile, `{"custom":true}`)
	beforeMagpie{"fork.new.absent", []string{newFile}}.take()
	if _, err := os.Stat(newFile + ".before-magpie"); !os.IsNotExist(err) {
		t.Fatalf("new recovery copy created: %v", err)
	}
}

func TestAstrBotConnectionCreatesNoRecoveryCopy(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "legacy"}[legacy], func(t *testing.T) {
			home, path := astrbotHome(t, astrbotConfig)
			if legacy {
				writeFile(t, path+".before-magpie", "older user data")
			}
			a := astrbot(home)
			for range 2 {
				if err := a.Pick("model", "magpie/kimi/k2"); err != nil {
					t.Fatal(err)
				}
				if err := a.Disconnect(); err != nil {
					t.Fatal(err)
				}
			}
			if legacy {
				if b, err := os.ReadFile(path + ".before-magpie"); err != nil || string(b) != "older user data" {
					t.Fatalf("legacy recovery changed: %q, %v", b, err)
				}
			} else if _, err := os.Stat(path + ".before-magpie"); !os.IsNotExist(err) {
				t.Fatalf("new recovery copy created: %v", err)
			}
		})
	}
}
