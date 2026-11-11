package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/settings"
)

func TestPartnerTelemetryNotCollected(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("DO_NOT_TRACK", "0")
	t.Setenv("MAGPIE_NO_STATS", "false")
	if err := settings.Save(settings.Settings{}); err != nil {
		t.Fatal(err)
	}
	cachedPartners(t)
	for _, event := range []string{PartnerShown, PartnerOpened, PartnerKeys, PartnerAdded} {
		CountPartner(event, "acme")
	}
	if _, err := os.Stat(filepath.Join(appdir.Cache(), "partner-counts.json")); !os.IsNotExist(err) {
		t.Errorf("partner telemetry was collected: %v", err)
	}
}
