package agent

import (
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// A malformed magpie-models.json is left intact by sync and disconnect,
// with no new recovery copies. Disconnect may still remove the wiring.
func TestUnparsedCodexCatalogLeftIntact(t *testing.T) {
	home, _ := codexHome(t, "", "")
	list := filepath.Join(home, ".codex", "magpie-models.json")
	cx := codex(home)
	if err := cx.Fields[0].Set("fake/m1"); err != nil {
		t.Fatal(err)
	}
	// the file as it stands, broken the way a half-save or a sync conflict
	// leaves one: a cut before the last brace, no longer a catalog
	b := readFile(list)
	writeFile(t, list, b[:len(b)-2])

	// a provider added: the sync writes the list again
	if err := provider.Save(provider.Provider{ID: "added", Name: "Added", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m2"}}); err != nil {
		t.Fatal(err)
	}
	SyncCatalog()
	if readFile(list) != string(b[:len(b)-2]) {
		t.Fatal("sync overwrote damaged catalog")
	}
	_ = cx.Fields[0].Set("")
	if readFile(list) != string(b[:len(b)-2]) {
		t.Fatal("damaged catalog changed")
	}
	if bad, _ := filepath.Glob(list + ".bad-*"); len(bad) != 0 {
		t.Fatal("recovery copies generated")
	}
}
