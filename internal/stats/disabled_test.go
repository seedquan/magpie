package stats

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/settings"
)

type recordingTransport struct{ requests int }

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.requests++
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
}

// Old preferences and environment overrides cannot enable telemetry in this fork.
func TestTelemetryAlwaysDisabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, value := range []string{"", "0", "false", "1"} {
		t.Run("environment="+value, func(t *testing.T) {
			t.Setenv("DO_NOT_TRACK", value)
			t.Setenv("MAGPIE_NO_STATS", value)
			if err := settings.Save(settings.Settings{NoStats: false, NoUsageStats: false}); err != nil {
				t.Fatal(err)
			}
			if !Off() {
				t.Fatal("third-party telemetry can be enabled")
			}
		})
	}
}

// Call the sender directly, with collection allowed by legacy settings and
// an overridden destination. All HTTP is intercepted, including the default host.
func TestTelemetrySendsNothingAndCreatesNoIdentity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("DO_NOT_TRACK", "0")
	t.Setenv("MAGPIE_NO_STATS", "false")
	t.Setenv("MAGPIE_STATS_HOST", "https://collector.example")
	if err := settings.Save(settings.Settings{}); err != nil {
		t.Fatal(err)
	}
	transport := &recordingTransport{}
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() { http.DefaultClient = old })
	day := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	for _, kind := range []string{"app", "serve"} {
		for _, at := range []time.Time{day, day.Add(24 * time.Hour)} {
			if err := Send(context.Background(), "v0.1.3408", kind, at); err != nil {
				t.Fatal(err)
			}
		}
	}
	if transport.requests != 0 {
		t.Errorf("telemetry made %d HTTP requests", transport.requests)
	}
	for _, name := range []string{"install-id", "stats-sent"} {
		if _, err := os.Stat(filepath.Join(settings.Dir(), name)); !os.IsNotExist(err) {
			t.Errorf("telemetry created %s: %v", name, err)
		}
	}
}
