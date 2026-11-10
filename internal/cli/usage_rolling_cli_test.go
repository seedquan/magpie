package cli

import (
	"strings"
	"testing"
	"time"

	stats "github.com/yetone/magpie/internal/usage"
)

// magpie usage --last and --days read a rolling window ending now (#2075):
// the local summary includes only calls in that window, where presets begin
// at a local midnight. The former CSV flags must not restore file export.
func TestUsageRollingWindow(t *testing.T) {
	groupsHome(t)
	now := holdUsageClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	stats.Append(stats.Record{Time: now.Add(-time.Hour), Agent: "codex", Provider: "a", Model: "m", Input: 10, Status: 200})
	stats.Append(stats.Record{Time: now.Add(-25 * time.Hour), Agent: "claude", Provider: "b", Model: "m", Input: 10, Status: 200})
	var b strings.Builder
	if err := usageTo(&b, []string{"usage", "--last", "24h"}); err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(b.String(), "\n")
	if !strings.Contains(head, "last 1 day") || !strings.Contains(head, "1 call") || !strings.Contains(b.String(), "a/m") || strings.Contains(b.String(), "b/m") {
		t.Fatalf("24-hour summary must show only the recent call:\n%s", b.String())
	}
	b.Reset()
	if err := usageTo(&b, []string{"usage", "--days", "1.5"}); err != nil {
		t.Fatal(err)
	}
	head, _, _ = strings.Cut(b.String(), "\n")
	if !strings.Contains(head, "last 36 hours") || !strings.Contains(head, "2 calls") || !strings.Contains(b.String(), "a/m") || !strings.Contains(b.String(), "b/m") {
		t.Fatalf("36-hour summary must include both calls:\n%s", b.String())
	}
	for _, window := range [][]string{{"--last", "24h"}, {"--days", "1.5"}} {
		b.Reset()
		args := append([]string{"usage", "--csv"}, window...)
		if err := usageTo(&b, args); err == nil || b.Len() != 0 {
			t.Errorf("rolling window restored CSV export: %v, output %q, error %v", args, b.String(), err)
		}
	}
}

func TestUsageRollingWindowRejectsJunk(t *testing.T) {
	groupsHome(t)
	for _, args := range [][]string{
		{"usage", "--last", "abc"},
		{"usage", "--last", "0s"},
		{"usage", "--days", "0"},
		{"usage", "--days", "-1"},
		{"usage", "--days"},
		{"usage", "--last"},
		{"usage", "--last", "24h", "--days", "1"},
		{"usage", "--last", "24h", "7d"}, // which window was meant can't be told
		{"usage", "--days", "NaN"},
		{"usage", "--days", "Inf"},
		{"usage", "--days", "1e30"},
	} {
		if err := usageTo(&strings.Builder{}, args); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
}

// --last keeps the local summary's account breakdown within the window.
// Account names remain viewable without the former account CSV export flag.
func TestUsageRollingWindowWithAccount(t *testing.T) {
	groupsHome(t)
	now := holdUsageClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	stats.Append(stats.Record{Time: now.Add(-time.Hour), Agent: "codex", Provider: "a", ProviderAccount: "me@x", Model: "m", Input: 10, Status: 200})
	stats.Append(stats.Record{Time: now.Add(-2 * time.Hour), Agent: "codex", Provider: "a", ProviderAccount: "other@x", Model: "m", Input: 10, Status: 200})
	stats.Append(stats.Record{Time: now.Add(-30 * time.Hour), Agent: "codex", Provider: "a", ProviderAccount: "older@x", Model: "old-request", Input: 10, Status: 200})
	var b strings.Builder
	if err := usageTo(&b, []string{"usage", "--last", "24h"}); err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(b.String(), "\n")
	if !strings.Contains(head, "last 1 day") || !strings.Contains(head, "2 calls") {
		t.Fatalf("account summary has the wrong window or count:\n%s", b.String())
	}
	if s := b.String(); !strings.Contains(s, "accounts") || !strings.Contains(s, "me@x") || !strings.Contains(s, "other@x") || strings.Contains(s, "older@x") || strings.Contains(s, "old-request") {
		t.Fatalf("account breakdown must include only calls within the rolling window:\n%s", s)
	}
}
