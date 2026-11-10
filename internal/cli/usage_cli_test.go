package cli

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
	stats "github.com/yetone/magpie/internal/usage"
)

// holdUsageClock stops the usage clock at at for the test and gives at back,
// for the test to stamp its calls by: midnight then never falls between a
// call and the period it is asked for in.
func holdUsageClock(t *testing.T, at time.Time) time.Time {
	t.Helper()
	old := stats.Clock
	stats.Clock = func() time.Time { return at }
	t.Cleanup(func() { stats.Clock = old })
	return at
}

// midnightUsageClock sets the usage clock to read a tenth of a second before
// midnight the first time and a tenth of a second after it from then on, as
// when midnight falls while magpie usage is worked out.
func midnightUsageClock(t *testing.T, midnight time.Time) {
	t.Helper()
	var read atomic.Bool
	old := stats.Clock
	stats.Clock = func() time.Time {
		if read.Swap(true) {
			return midnight.Add(100 * time.Millisecond)
		}
		return midnight.Add(-100 * time.Millisecond)
	}
	t.Cleanup(func() { stats.Clock = old })
}

// The local usage summary names upstream provider keys without exposing them.
func TestUsageProviderKeys(t *testing.T) {
	groupsHome(t)
	id := provider.KeyID("fixture-provider-secret")
	stats.Append(stats.Record{Provider: "relay", Model: "m", ProviderKeyID: id, ProviderKeyName: "Team", Input: 30, Status: 200})
	out, err := said(t, func() error { return usageCmd([]string{"usage", "all"}) })
	if err != nil || !strings.Contains(out, "upstream provider keys") || !strings.Contains(out, "relay / Team") || strings.Contains(out, "fixture-provider-secret") {
		t.Fatal(out, err)
	}

}

// magpie usage shows, beside the gateway's calls, those the agents made on
// their own, read from their session files, as the window's Usage page
// does (Kumo31 on Discord: Codex used outside magpie wasn't in it).
func TestUsageShowsCallsNotThroughMagpie(t *testing.T) {
	groupsHome(t)
	now := holdUsageClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	stats.Append(stats.Record{Time: now.Add(-time.Minute), Agent: "claude", Provider: "relay", Model: "m", Input: 5, Status: 200})
	old := stats.LogCalls
	stats.LogCalls = func(time.Time) []sessions.Call {
		return []sessions.Call{{Time: now.Add(-2 * time.Minute), Agent: "codex", Session: "c1", Model: "gpt-6-luna", Tokens: sessions.Tokens{Input: 1200, Output: 300}}}
	}
	t.Cleanup(func() { stats.LogCalls = old })
	var b strings.Builder
	if err := usageTo(&b, []string{"usage", "today"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	gw, own, ok := strings.Cut(out, "not through magpie")
	if !ok || !strings.Contains(gw, "relay/m") || strings.Contains(gw, "gpt-6-luna") || !strings.Contains(own, "gpt-6-luna") || !strings.Contains(own, "1.5K") {
		t.Fatal(out)
	}

	// a call of the agents' own from before this month shows under all
	stats.LogCalls = func(time.Time) []sessions.Call {
		return []sessions.Call{{Time: now.AddDate(0, 0, -40), Agent: "codex", Model: "gpt-6-luna", Tokens: sessions.Tokens{Input: 7}}}
	}
	b.Reset()
	if err := usageTo(&b, []string{"usage", "all"}); err != nil || !strings.Contains(b.String(), "not through magpie") || !strings.Contains(b.String(), "gpt-6-luna") {
		t.Fatal(b.String(), err)
	}
}

// magpie usage asked as midnight falls shows the calls through magpie and
// those not through it of one day, the day it was asked on. Asked apart,
// the calls not through magpie were the next day's, none yet, and left out.
func TestUsageAskedAtMidnight(t *testing.T) {
	groupsHome(t)
	midnight := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	stats.Append(stats.Record{Time: midnight.Add(-time.Minute), Agent: "claude", Provider: "relay", Model: "m", Input: 5, Status: 200})
	old := stats.LogCalls
	stats.LogCalls = func(time.Time) []sessions.Call {
		return []sessions.Call{{Time: midnight.Add(-2 * time.Minute), Agent: "codex", Session: "c1", Model: "gpt-6-luna", Tokens: sessions.Tokens{Input: 1200, Output: 300}}}
	}
	t.Cleanup(func() { stats.LogCalls = old })
	midnightUsageClock(t, midnight)
	var b strings.Builder
	if err := usageTo(&b, []string{"usage", "today"}); err != nil {
		t.Fatal(err)
	}
	gw, own, ok := strings.Cut(b.String(), "not through magpie")
	if !ok || !strings.Contains(gw, "relay/m") || !strings.Contains(own, "gpt-6-luna") {
		t.Fatal(b.String())
	}
}

// magpie usage's headline says how many calls its cost leaves out for
// having no known price, beside the sum, not only a "+" after it (#1856):
// a Kimi K3 Auto that nobody prices is never read as costing nothing.
func TestUsageSaysCallsUnpriced(t *testing.T) {
	groupsHome(t)
	day := stats.Today.Since(holdUsageClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)))
	if err := provider.SetModelPrice("a/m", &catalog.Price{Input: 1, Output: 2}); err != nil {
		t.Fatal(err)
	}
	stats.Append(stats.Record{Time: day.Add(time.Second), Agent: "codex", Provider: "a", Model: "m", Input: 1000, Output: 100, Status: 200})
	for i := range 2 {
		stats.Append(stats.Record{Time: day.Add(time.Duration(2+i) * time.Second), Agent: "codex", Provider: "a", Model: "only-a", Input: 1000, Output: 100, Status: 200})
	}
	var b strings.Builder
	if err := usageTo(&b, []string{"usage", "today"}); err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(b.String(), "\n")
	if !strings.Contains(head, "≈$0.001+") || !strings.Contains(head, "(2 calls unpriced)") {
		t.Fatalf("headline %q", head)
	}
}

// magpie usage says what the requests cost in their plans' credits where
// the replies said (WorkBuddy's usage.credit, #2512): in the summary, and
// on each agent's, model's and session's row of them; nothing where none did.
func TestUsageSaysCredits(t *testing.T) {
	groupsHome(t)
	day := stats.Today.Since(holdUsageClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)))
	c := 0.35
	for i := 0; i < 2; i++ {
		stats.Append(stats.Record{Time: day.Add(time.Duration(i+1) * time.Second), Agent: "codex", Session: "s1", Provider: "workbuddy-ai", Model: "gemini-3.5-flash", Input: 10, Output: 2, Status: 200, Credits: &c})
	}
	stats.Append(stats.Record{Time: day.Add(5 * time.Second), Agent: "pi", Session: "s2", Provider: "a", Model: "m", Input: 1, Output: 1, Status: 200})
	var b strings.Builder
	if err := usageTo(&b, []string{"usage", "today"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if n := strings.Count(out, "0.7 credits"); n < 4 { // the summary, the agent, the model and the session
		t.Fatalf("0.7 credits said %d times:\n%s", n, out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "pi") && strings.Contains(l, "credits") {
			t.Fatalf("a row with no credits said says some: %q", l)
		}
	}
}
