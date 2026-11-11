package gateway

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDamagedSightCacheStopsWritesAcrossInstances(t *testing.T) {
	s, u := eyed(t)
	other := New() // made before corruption; a changed file is revalidated
	if err := os.MkdirAll(filepath.Dir(s.sightFile), 0o700); err != nil {
		t.Fatal(err)
	}
	const damaged = `{"version":2,"kind":"inline","key":"truncated`
	if err := os.WriteFile(s.sightFile, []byte(damaged), 0o600); err != nil {
		t.Fatal(err)
	}
	var messages bytes.Buffer
	writer := log.Writer()
	log.SetOutput(&messages)
	t.Cleanup(func() { log.SetOutput(writer) })
	for i, server := range []*Server{s, other, New(), New()} {
		for range 2 {
			if code, got := postAs(t, server, "", imagesBody(1)); code != 200 {
				t.Fatalf("instance %d with damaged persistence: %d %s", i, code, got)
			}
		}
		if server.sightDiskLines != sightsDiskDisabled {
			t.Fatalf("instance %d continues persistent saves", i)
		}
		if got, err := os.ReadFile(s.sightFile); err != nil || string(got) != damaged {
			t.Fatalf("instance %d changed damaged bytes: %q, %v", i, got, err)
		}
	}
	if got := len(u.sent("eye")); got != 4 {
		t.Fatalf("memory cache stopped working: %d descriptions, want one per instance", got)
	}
	if n := strings.Count(messages.String(), "persistence disabled"); n != 4 ||
		!strings.Contains(messages.String(), "repair or move it explicitly") {
		t.Fatalf("repair advice should be logged once per instance, got %q", messages.String())
	}
	if copies, err := filepath.Glob(s.sightFile + ".broken*"); err != nil || len(copies) != 0 {
		t.Fatalf("recovery copies generated: %v, %v", copies, err)
	}
}

func TestOversizedSightCacheReadIsBounded(t *testing.T) {
	fresh(t)
	path := sightsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse data models a file a damaged appender allowed to keep growing.
	if err := f.Truncate(2 * sightsMaxDiskBytes); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		s := New()
		for i := range 3 {
			s.saveSight(fmt.Sprintf("%064x", i+1), "still usable in memory", "probe/eye")
		}
		if s.sightDiskLines != sightsDiskDisabled {
			t.Fatal("oversized file did not disable persistence")
		}
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 2*sightsMaxDiskBytes {
		t.Fatalf("oversized file grew or was replaced: %v, %v", info, err)
	}
}
