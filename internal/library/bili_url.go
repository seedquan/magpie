package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/agent"
)

// Zed has no plugin of bili's. Its README lists it with two routes: the
// launcher (bili zed), Linux only and leaving a provider on loopback
// direct, so magpie's gateway never passes through it; and, on every
// system, the /bili/ prefix its docs give for any client whose model
// address can be edited ("Adopting unlisted clients", docs/clients.md):
// language_models.openai_compatible.<provider>.api_url as
// http://127.0.0.1:8787/bili/<the address>, with bili's daemon (bili start)
// answering there. That is what magpie writes into Zed's magpie provider
// (agent.ZedThrough, which keeps it across picks and syncs and saves the
// key for it, Zed looking that up by the address). This fork only uses a
// daemon the user already started; magpie never manages its process.

// biliOrigin is where bili start listens: 8787, the port bili keeps for
// it (its lanes take 18787 and up).
const biliOrigin = "http://127.0.0.1:8787"

// biliPrefix is what goes before the gateway's address.
const biliPrefix = biliOrigin + "/bili/"

// biliURLLane is an agent bili folds through its /bili/ prefix: whether
// its magpie provider goes through it, and putting it there (prefix
// biliPrefix) or back on the gateway ("").
type biliURLLane struct {
	on  func(a *agent.Agent) bool
	set func(a *agent.Agent, prefix string) error
}

// zedThrough is agent.ZedThrough; a var for tests, which save no key.
var zedThrough = agent.ZedThrough

// Hermes has a plugin of bili's (bili plugin install hermes), but it
// routes Hermes's traffic by setting HTTPS_PROXY in Hermes's process
// alone (hermes-plugin/__init__.py apply_env), and magpie's provider is at
// http://127.0.0.1, which that never takes: the plugin would add its
// tools while nothing folds. So Hermes goes through the /bili/ prefix as
// Zed does: agent.HermesThrough puts the base_url of magpie's entries, in
// every profile that has them, behind it, which a sync and a pick keep
// (Duckb7si on X).

var biliURLLanes = map[string]biliURLLane{
	"zed": {
		on:  func(a *agent.Agent) bool { return strings.HasPrefix(agent.ZedURL(a), biliPrefix) },
		set: func(a *agent.Agent, prefix string) error { return zedThrough(a, prefix) },
	},
	"hermes": {
		on:  func(a *agent.Agent) bool { return strings.HasPrefix(agent.HermesURL(a), biliPrefix) },
		set: agent.HermesThrough,
	},
}

// biliServing says bili start answers at biliOrigin: its health is
// {"ok":true}, which something else on the port doesn't say. A var for
// tests.
var biliServing = func() bool {
	c := &http.Client{Timeout: 2 * time.Second}
	res, err := c.Get(biliOrigin + "/__bili/health")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var h struct {
		OK bool `json:"ok"`
	}
	return res.StatusCode == http.StatusOK && json.NewDecoder(res.Body).Decode(&h) == nil && h.OK
}

// biliServe only reuses an existing daemon. Launching the external CLI
// could start its own updater; daemon lifecycle stays with the user.
// A var for tests.
var biliServe = func(_ string) error {
	if biliServing() {
		return nil
	}
	return fmt.Errorf("billion-context is not answering at %s; start it yourself before enabling this connection", biliOrigin)
}

// biliStop never stops a daemon owned by the user. A var for tests.
var biliStop = func() {}

// biliURLOn is whether any agent here goes through bili's prefix.
func biliURLOn(agents []*agent.Agent) bool {
	for _, a := range biliAgents(agents) {
		if l, ok := biliURLLanes[a.ID]; ok && l.on(a) {
			return true
		}
	}
	return false
}

// setBiliURL puts an agent's magpie provider through an existing daemon's
// prefix, or back on the gateway. The daemon remains owned by the user.
func setBiliURL(a *agent.Agent, agents []*agent.Agent, bin string, on bool) (*BiliView, error) {
	b := newBackups()
	if err := b.keep(a.ID, a.Path); err != nil {
		return nil, err
	}
	l := biliURLLanes[a.ID]
	if on {
		if err := biliServe(bin); err != nil {
			return nil, err
		}
		if err := l.set(a, biliPrefix); err != nil {
			return nil, err
		}
	} else {
		if err := l.set(a, ""); err != nil {
			return nil, err
		}
		if !biliURLOn(agents) {
			biliStop()
		}
	}
	v := readBili(agents)
	v.Restart, v.Backup = []string{a.ID}, b.dir
	if b.dir != "" {
		pruneBackups()
	}
	return v, nil
}

// keepBiliServe is disabled: startup and periodic work never launch or
// restart an external daemon that may manage its own updates.
func keepBiliServe(context.Context) {}
