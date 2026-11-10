package provider

import (
	"os"
	"path/filepath"
	"sync/atomic"
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
	resetPartners()
	t.Cleanup(resetPartners)
	var body atomic.Value
	body.Store(`{"partners":[{"id":"acme","name":"Acme","chat":"https://a.example/v1"}]}`)
	t.Setenv("MAGPIE_PARTNERS", partnerFeedServer(t, &body).URL)
	fetchPartnersNow()
	for _, event := range []string{PartnerShown, PartnerOpened, PartnerKeys, PartnerAdded} {
		CountPartner(event, "acme")
	}
	if _, err := os.Stat(filepath.Join(appdir.Cache(), "partner-counts.json")); !os.IsNotExist(err) {
		t.Errorf("partner telemetry was collected: %v", err)
	}
}
