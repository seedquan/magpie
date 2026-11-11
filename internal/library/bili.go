package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/jsonc"
	"github.com/yetone/magpie/internal/agent"
	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/lazyre"
	"github.com/yetone/magpie/internal/proc"
	"github.com/yetone/magpie/internal/settings"
)

// billion-context (npm billion-context, its CLI bili) compresses an
// agent's context as it runs: old turns are folded into summaries the
// model can open again (compress, decompress, search_context), so a long
// session stays under the window. Its native plugins are written into an
// agent's own files by its own installer (bili plugin install <agent>).
// That installer creates persistent recovery copies, so this fork refuses
// native installs. A recorded legacy install can be undone locally.
//
// Only the agents where bili's plugin works with magpie in front are
// offered: Pi, omp and OpenCode load it in process, and their requests
// still go to magpie with bili's tools added; Claude Code's goes through
// bili's local proxy, which forwards to the gateway (ANTHROPIC_BASE_URL
// .../bili/<magpie's address>). Codex's lane is an MCP server only, ZCode's
// routes only its own provider, Kimi Code's renames the default model in
// config.toml at each start (bili-kimi) and dsh's installs through dsh's
// package manager: none of those is offered.
//
// Zed has no plugin of bili's, and Hermes' sets HTTPS_PROXY alone, which
// magpie's http://127.0.0.1 provider never takes: their magpie provider
// goes through bili's /bili/ prefix with bili start answering there
// (biliURLLanes, bili_url.go).
//
// bili's remove doesn't put a file back as it was: it leaves "packages": []
// in Pi's settings.json and "extensions:" in omp's config.yml, and after
// magpie rewrote Claude Code's settings.json it took ANTHROPIC_BASE_URL out
// altogether (the agent then went to Anthropic) and put back the
// DISABLE_AUTO_COMPACT it had added. Legacy records retain each file before
// and after the install. Local undo restores only what is still known to
// be the install's change (biliRestore), leaving the old records untouched.

// BiliURL is billion-context's page.
const BiliURL = "https://github.com/ranxianglei/billion-context"

// biliPackage is its npm package.
const biliPackage = "billion-context"

// biliLanes are the agents (magpie's ids) bili's plugin is offered for,
// each with the files its install writes that magpie puts back.
var biliLanes = map[string]func(a *agent.Agent) []string{
	"pi":       func(a *agent.Agent) []string { return []string{a.Path} },
	"omp":      func(a *agent.Agent) []string { return []string{a.Path} },
	"opencode": func(a *agent.Agent) []string { return []string{a.Path} },
	"claude": func(a *agent.Agent) []string {
		return []string{a.Path, filepath.Join(filepath.Dir(a.Path), "commands", "acp-cache.md")}
	},
}

// BiliAgent is one agent bili's plugin can be given to.
type BiliAgent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	On   bool   `json:"on"`
	// Status is what bili plugin list says of it when that isn't simply
	// installed or not: partly installed, broken, an error
	Status string `json:"status,omitempty"`
}

// BiliView is the billion-context part of the Library page.
type BiliView struct {
	Path    string      `json:"path,omitempty"` // "" when bili isn't installed
	Version string      `json:"version,omitempty"`
	Agents  []BiliAgent `json:"agents"`
	URL     string      `json:"url"`
	// Install is the command Install runs, "" when there is no npm
	Install string `json:"install,omitempty"`
	// ListErr is why bili plugin list said nothing magpie could read
	ListErr string   `json:"listErr,omitempty"`
	Restart []string `json:"restart,omitempty"`
	Backup  string   `json:"backup,omitempty"`
	// Saved is what bili says it saved (bili_saved.go), nil when no bili
	// has answered here yet
	Saved *BiliSaved `json:"saved,omitempty"`
}

var biliMu sync.Mutex

// biliPath is bili's, "" when it isn't installed: on PATH, or where its
// documented install (npm -g with ~/.local as the prefix) puts it, which
// may not be on magpie's PATH yet.
var biliPath = func() string {
	name := "bili"
	if runtime.GOOS == "windows" {
		name = "bili.cmd"
	}
	if p, err := proc.Look(name); err == nil {
		return p
	}
	dirs := []string{filepath.Join(home(), ".local", "bin")}
	if runtime.GOOS == "windows" {
		dirs = []string{filepath.Join(appdir.Getenv("APPDATA"), "npm")}
	}
	for _, d := range dirs {
		if p := filepath.Join(d, name); exists(p) {
			return p
		}
	}
	return ""
}

// biliInstaller is billion-context's documented install: npm's global
// install with ~/.local as the prefix, so it needs no administrator; on
// Windows npm's own global folder, which is the user's. nil without npm.
var biliInstaller = func() []string {
	npm, err := proc.Look("npm")
	if err != nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		return []string{npm, "install", "-g", biliPackage}
	}
	return []string{npm, "install", "-g", biliPackage, "--prefix=" + filepath.Join(home(), ".local")}
}

// biliRun runs bili with nothing to answer it.
var biliRun = func(bin string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := proc.CommandContext(ctx, bin, args...)
	cmd.Stdin = nil
	// a bili in ~/.local/bin is a node script: its folder goes first, and
	// node is found where the user's shell finds it
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return text, fmt.Errorf("bili %s didn't finish in %s", strings.Join(args, " "), timeout)
		}
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("bili %s: %s", strings.Join(args, " "), lastLines(text, 4))
	}
	return text, nil
}

// biliAgents are the agents here bili's plugin can be given to. One in
// WSL isn't: bili there is the distro's, and magpie runs this machine's.
func biliAgents(agents []*agent.Agent) []*agent.Agent {
	if agents == nil {
		agents = detected()
	}
	var out []*agent.Agent
	for _, a := range agents {
		_, plugin := biliLanes[a.ID]
		if _, url := biliURLLanes[a.ID]; (plugin || url) && a.WSL == "" && !a.CLIMissing() {
			out = append(out, a)
		}
	}
	return out
}

// biliStatusLine is a row of bili plugin list: the agent, then its
// status, then the version and how it updates, apart by two spaces or more.
var biliColumns = lazyre.New(`\s{2,}`)

// parseBiliList is each agent's status as bili plugin list prints it.
func parseBiliList(out string) map[string]string {
	st := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		cols := biliColumns.Split(strings.TrimSpace(line), -1)
		if len(cols) < 2 || cols[0] == "agent" {
			continue
		}
		st[cols[0]] = cols[1]
	}
	return st
}

// biliOn is whether a status says the plugin is there and whole.
func biliOn(status string) bool {
	return status == "installed" || strings.HasPrefix(status, "installed (")
}

// ReadBili says whether bili is installed and which agents have its plugin.
// biliVersion is the version in `bili --version`'s output. Its output is
// read with stderr, where bili writes warnings first (a config key it
// doesn't know: "2026-11-09T23:44:06.211 [warn] [v=0.1.213] ..."), so
// the version is the first word of the line that starts with one.
func biliVersion(out string) string {
	for _, l := range strings.Split(out, "\n") {
		w, _, _ := strings.Cut(strings.TrimSpace(l), " ")
		if biliVersionRe.MatchString(w) {
			return w
		}
	}
	return ""
}

var biliVersionRe = lazyre.New(`^v?\d+\.\d+\.\d+([-+][0-9A-Za-z.+-]*)?$`)

func ReadBili() *BiliView {
	return readBili(detected())
}

func readBili(agents []*agent.Agent) *BiliView {
	v := &BiliView{Agents: []BiliAgent{}, URL: BiliURL, Path: biliPath()}
	var st map[string]string
	if v.Path == "" {
		if c := biliInstaller(); c != nil {
			v.Install = biliShown(c)
		}
	} else {
		if out, err := biliRun(v.Path, 30*time.Second, "--version"); err == nil {
			v.Version = biliVersion(out)
		}
		v.Saved = readBiliSaved()
		out, err := biliRun(v.Path, 30*time.Second, "plugin", "list")
		if st = parseBiliList(out); err != nil || len(st) == 0 {
			v.ListErr = lastLines(out, 2)
			if err != nil {
				v.ListErr = err.Error()
			}
		}
	}
	for _, a := range biliAgents(agents) {
		ba := BiliAgent{ID: a.ID, Name: a.Name, Icon: a.Icon}
		if l, ok := biliURLLanes[a.ID]; ok {
			ba.On = l.on(a)
		} else if s, ok := st[a.ID]; ok {
			ba.On = biliOn(s)
			if !ba.On && s != "not installed" {
				ba.Status = s
			}
		}
		v.Agents = append(v.Agents, ba)
	}
	return v
}

// biliShown is the install command as the page shows it: npm by name.
func biliShown(c []string) string {
	c = slices.Clone(c)
	c[0] = strings.TrimSuffix(filepath.Base(c[0]), ".cmd")
	return strings.Join(c, " ")
}

// biliFile is a legacy record of each file bili's install wrote, as it was
// before (nil: not there) and as the install left it. No new records are made.
type biliFile struct {
	Path   string  `json:"path"`
	Before *string `json:"before"`
	After  *string `json:"after"`
	// Marks are old .bili-bak and .bili-last paths. They are never removed.
	Marks []string `json:"marks,omitempty"`
}

func biliStateFile() string { return filepath.Join(settings.Dir(), "bili-plugins.json") }

func readBiliState() (map[string][]biliFile, error) {
	m := map[string][]biliFile{}
	b, err := os.ReadFile(biliStateFile())
	if err != nil {
		return nil, fmt.Errorf("cannot read legacy billion-context state; repair it explicitly before disconnecting: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("cannot parse legacy billion-context state; repair it explicitly before disconnecting: %w", err)
	}
	return m, nil
}

func biliRead(path string) (*string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}

// SetBili switches URL lanes, refuses native installs, and undoes recorded
// legacy native installs locally without running their installer or remover.
func SetBili(id string, on bool) (*BiliView, error) {
	biliMu.Lock()
	defer biliMu.Unlock()
	agents := detected()
	var a *agent.Agent
	for _, x := range biliAgents(agents) {
		if x.ID == id {
			a = x
		}
	}
	files, ok := biliLanes[id]
	_, url := biliURLLanes[id]
	switch {
	case !ok && !url:
		return nil, fmt.Errorf("billion-context isn't offered for %s", id)
	case a == nil:
		return nil, fmt.Errorf("%s isn't installed", id)
	}
	if url {
		bin := biliPath()
		if bin == "" {
			return nil, fmt.Errorf("billion-context isn't installed — install it first (%s)", BiliURL)
		}
		return setBiliURL(a, agents, bin, on)
	}
	if on {
		return nil, errors.New("native billion-context plugin installation is disabled because its installer creates persistent recovery copies")
	}
	state, err := readBiliState()
	if err != nil {
		return nil, err
	}
	snap := state[id]
	if len(snap) == 0 {
		return nil, fmt.Errorf("no legacy billion-context install record for %s; remove its native integration explicitly without running the recovery-copy installer/remover", id)
	}
	paths := files(a)
	cur, desired := map[string]*string{}, map[string]*string{}
	for _, p := range paths {
		var f *biliFile
		for i := range snap {
			if snap[i].Path == p {
				if f != nil {
					return nil, fmt.Errorf("duplicate legacy billion-context records for %s; repair the state explicitly", p)
				}
				f = &snap[i]
			}
		}
		if f == nil {
			return nil, fmt.Errorf("no legacy billion-context install record for %s; repair the integration explicitly", p)
		}
		if p == a.Path && f.After == nil {
			return nil, fmt.Errorf("legacy billion-context record for %s has no installed configuration; repair the integration explicitly", p)
		}
		cur[p], err = biliRead(p)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s before undoing billion-context: %w", p, err)
		}
		desired[p], err = biliRestore(p, f, cur[p], cur[p])
		if err != nil {
			return nil, fmt.Errorf("cannot safely undo billion-context in %s; repair the integration explicitly: %w", p, err)
		}
	}
	// Compute every result before writing any file; a later unreadable or
	// damaged file must leave the earlier files alone too.
	if err := edit.Atomically(func() error {
		for _, p := range paths {
			now, err := biliRead(p)
			if err != nil {
				return err
			}
			if !same(now, cur[p]) {
				return fmt.Errorf("%s changed during billion-context undo; retry explicitly", p)
			}
		}
		for _, p := range paths {
			out := desired[p]
			if same(out, cur[p]) {
				continue
			}
			if out == nil {
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					return err
				}
			} else if err := edit.WriteAtomic(p, []byte(*out)); err != nil {
				return err
			}
		}
		return nil
	}, paths...); err != nil {
		return nil, err
	}
	v := readBili(agents)
	v.Restart = []string{id}
	return v, nil
}

// keepBiliCompact no longer rewrites legacy whole-file snapshots at startup.
func keepBiliCompact() {}

// biliRestore computes the undo without writing: only installed values
// unchanged since the recorded install revert. Local undo passes cur as
// now too, since no external remover is run.
func biliRestore(p string, f *biliFile, cur, now *string) (*string, error) {
	if f == nil {
		return nil, errors.New("no legacy install record")
	}
	if same(f.Before, f.After) || same(cur, f.Before) {
		return cur, nil
	}
	if cur == nil && f.After != nil {
		return nil, errors.New("the installed file is missing")
	}
	var out *string
	isJSON := strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".jsonc")
	if isJSON {
		if cur != nil && strings.TrimSpace(*cur) == "" {
			return nil, errors.New("not JSON")
		}
		for _, s := range []*string{cur, f.Before, f.After} {
			if _, ok := parseJSONC(s); !ok {
				return nil, errors.New("not JSON")
			}
		}
	}
	switch {
	case same(cur, f.After):
		// nothing touched it since the install: as it was before, byte for byte
		out = f.Before
	case isJSON:
		s, err := biliMergeJSON(f.Before, f.After, cur, now)
		if err != nil {
			return nil, err
		}
		out = s
		if f.Before != nil && out != nil && sameJSON(*out, *f.Before) {
			out = f.Before
		}
	default:
		return nil, errors.New("the file has changes outside the recorded install")
	}
	if isJSON {
		// A conflict or an unpatchable key can leave an installed value
		// active. Prove the whole plan undoes it before any file is written.
		before, _ := parseJSONC(f.Before)
		after, _ := parseJSONC(f.After)
		planned, ok := parseJSONC(out)
		if !ok {
			return nil, errors.New("the planned undo is not JSON")
		}
		var installed [][]string
		jsonDiffs(before, after, nil, &installed)
		for _, path := range installed {
			want, had := jsonAt(before, path)
			got, has := jsonAt(planned, path)
			if had != has || !jsonEqual(want, got) {
				return nil, errors.New("an installed value cannot be safely restored; manual repair is required before disconnecting")
			}
		}
	}
	return out, nil
}

func same(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func parseJSONC(s *string) (map[string]any, bool) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return map[string]any{}, true
	}
	var m map[string]any
	if json.Unmarshal(jsonc.ToJSON(edit.TrimBOM([]byte(*s))), &m) != nil || m == nil {
		return nil, false
	}
	return m, true
}

func sameJSON(a, b string) bool {
	x, ok1 := parseJSONC(&a)
	y, ok2 := parseJSONC(&b)
	return ok1 && ok2 && jsonEqual(x, y)
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// jsonAt is the value at path in m, and whether there is one.
func jsonAt(m map[string]any, path []string) (any, bool) {
	var v any = m
	for _, k := range path {
		o, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = o[k]; !ok {
			return nil, false
		}
	}
	return v, true
}

// jsonDiffs are the paths where a and b differ: down through objects both
// have, an object, array or value that differs as a whole below that.
func jsonDiffs(a, b map[string]any, at []string, out *[][]string) {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	for k := range keys {
		p := append(slices.Clone(at), k)
		x, okx := a[k]
		y, oky := b[k]
		xm, isx := x.(map[string]any)
		ym, isy := y.(map[string]any)
		switch {
		case okx && oky && isx && isy:
			jsonDiffs(xm, ym, p, out)
		case okx != oky || !jsonEqual(x, y):
			*out = append(*out, p)
		}
	}
}

func under(p, q []string) bool {
	return len(p) >= len(q) && slices.Equal(p[:len(q)], q)
}

// keyPath is p as edit's key paths spell it, false when a key has a
// character they can't hold.
func keyPath(p []string) (string, bool) {
	parts := make([]string, len(p))
	for i, k := range p {
		if k == "" || strings.ContainsAny(k, `*?|#@\!=<>%`) {
			return "", false
		}
		parts[i] = strings.ReplaceAll(k, ".", `\.`)
	}
	return strings.Join(parts, "."), true
}

// biliURL is the ANTHROPIC_BASE_URL bili's Claude Code plugin writes: its
// proxy, then the URL it forwards to.
var biliURL = lazyre.New(`^https?://(?:127\.0\.0\.1|localhost|\[::1\]):\d+/bili/(https?://.+)$`)

// biliMergeJSON is now with what the install changed (before → after) and
// nothing changed since (cur still has it) put back as before, and what
// the remove changed beyond that (cur → now) put back as cur. Without a
// record of the install, a Claude Code base URL the remove took out goes
// back to the address bili forwarded to.
func biliMergeJSON(before, after, cur, now *string) (*string, error) {
	B, ok1 := parseJSONC(before)
	I, ok2 := parseJSONC(after)
	C, ok3 := parseJSONC(cur)
	R, ok4 := parseJSONC(now)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil, errors.New("not JSON")
	}
	type change struct {
		path []string
		v    any
		has  bool
	}
	var changes []change
	var installed [][]string
	if after != nil {
		jsonDiffs(B, I, nil, &installed)
	}
	for _, p := range installed {
		ci, hc := jsonAt(C, p)
		ii, hi := jsonAt(I, p)
		if hc == hi && jsonEqual(ci, ii) {
			v, has := jsonAt(B, p)
			changes = append(changes, change{p, v, has})
		}
	}
	if after == nil {
		// no record of the install: all the remove took out may be bili's,
		// except the address Claude Code was sent to before bili's proxy
		base := []string{"env", "ANTHROPIC_BASE_URL"}
		c, hc := jsonAt(C, base)
		_, hr := jsonAt(R, base)
		if s, ok := c.(string); ok && hc && !hr {
			if m := biliURL.FindStringSubmatch(s); m != nil {
				changes = append(changes, change{base, m[1], true})
			}
		}
	} else {
		var removed [][]string
		jsonDiffs(C, R, nil, &removed)
		for _, p := range removed {
			if slices.ContainsFunc(installed, func(q []string) bool { return under(p, q) || under(q, p) }) {
				continue
			}
			v, has := jsonAt(C, p)
			changes = append(changes, change{p, v, has})
		}
	}
	if len(changes) == 0 {
		return now, nil
	}
	raw := []byte("{}\n")
	if now != nil {
		raw = []byte(*now)
	}
	var set []edit.KV
	var del []string
	for _, c := range changes {
		kp, ok := keyPath(c.path)
		if !ok {
			continue
		}
		if !c.has {
			del = append(del, kp)
			continue
		}
		b, err := json.Marshal(c.v)
		if err != nil {
			return nil, err
		}
		set = append(set, edit.KV{Path: kp, Value: json.RawMessage(b)})
	}
	out, err := edit.PatchJSON(raw, set, del)
	if err != nil {
		return nil, err
	}
	// an object the remove emptied that wasn't there before goes too
	s := string(out)
	if now == nil && len(set) == 0 {
		return nil, nil
	}
	return &s, nil
}

// InstallBili installs billion-context the way its README says.
func InstallBili() (*BiliView, error) {
	biliMu.Lock()
	defer biliMu.Unlock()
	if biliPath() != "" {
		return ReadBili(), nil
	}
	c := biliInstaller()
	if c == nil {
		return nil, fmt.Errorf("npm isn't installed — install Node.js first, or see %s", BiliURL)
	}
	out, err := runPlain(c, 5*time.Minute)
	if err != nil {
		return nil, err
	}
	proc.Forget()
	v := ReadBili()
	if v.Path == "" {
		return nil, fmt.Errorf("%s ran, but bili still isn't found: %s", biliShown(c), lastLines(out, 3))
	}
	return v, nil
}
