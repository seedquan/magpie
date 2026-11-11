package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func partnerIDs(ps []Partner) string {
	var ids []string
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	return strings.Join(ids, ",")
}

func cachedPartners(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("MAGPIE_PARTNERS", "")
	resetPartners()
	t.Cleanup(resetPartners)
	ps := []Partner{
		{PresetDef: PresetDef{ID: "cached", Name: "Cached", Chat: "https://cached.example/v1"}},
		{PresetDef: PresetDef{ID: "old", Name: "Old", Chat: "https://old.example/v1"}, Until: time.Now().Add(-time.Hour)},
		{PresetDef: PresetDef{ID: "unsafe", Name: "Unsafe", Chat: "http://unsafe.example/v1"}},
	}
	seen := map[string]Partner{}
	for _, p := range ps {
		seen[p.ID] = p
	}
	b, _ := json.Marshal(partnerCache{Partners: ps, Seen: seen})
	os.MkdirAll(filepath.Dir(partnerCacheFile()), 0o700)
	if err := os.WriteFile(partnerCacheFile(), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPartnersFromLocalCache(t *testing.T) {
	cachedPartners(t)
	if got := partnerIDs(Partners()); got != "cached" {
		t.Fatalf("partners = %s", got)
	}
	if p := Preset("old"); p == nil || p.Sponsored || p.Chat != "https://old.example/v1" {
		t.Fatal("past provider lost its preset")
	}
	if p := Preset("unsafe"); p != nil {
		t.Fatal("unsafe cache preset accepted")
	}
	if p := Preset("cached"); p == nil || !p.Sponsored {
		t.Fatal("cached preset unavailable")
	}
	resetPartners()
	if got := partnerIDs(PartnersNow(time.Second)); got != "cached" {
		t.Fatalf("after restart = %s", got)
	}
}

func TestPartnersOff(t *testing.T) {
	cachedPartners(t)
	t.Setenv("MAGPIE_PARTNERS", "off")
	if len(Partners()) != 0 || len(PartnersNow(time.Second)) != 0 {
		t.Fatal("off listed cached partners")
	}
}

func TestNoticePartners(t *testing.T) {
	cachedPartners(t)
	NoticePartners("cached", "unknown")
	resetPartners()
	if !PartnerNoticed("cached") || PartnerNoticed("unknown") {
		t.Fatal("local notice not retained")
	}
}

func TestPartnersRejectUnsafeCachedFields(t *testing.T) {
	ps := validPartners([]Partner{{PresetDef: PresetDef{ID: "cached", Name: "Cached", Chat: "https://cached.example/v1", Decide: "https://cached.example/decide", NoKey: true}, IconURL: "https://cached.example/icon"}})
	if len(ps) != 1 || ps[0].Decide != "" || ps[0].NoKey || ps[0].Icon != "generic" {
		t.Fatal("unsafe remote-only fields retained")
	}
}
