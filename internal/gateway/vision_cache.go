package gateway

// Completed image descriptions are kept next to the rest of magpie's
// settings, so restarting or updating the gateway doesn't describe every
// image in an ongoing conversation again. Only inline images are saved:
// their cache key comes from their bytes. A URL is left to the in-memory
// cache because the bytes it points at can change.

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/filelock"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/steady"
)

const (
	sightCacheVersion  = 2
	sightKindInline    = "inline"
	sightsCompactAfter = 2 * sightsKept
	// A damaged legacy file is never allowed to make every restart read
	// an unbounded cache. Oversized files need the same explicit repair.
	sightsMaxDiskBytes = 32 << 20
	sightsDiskDisabled = -1
)

// sightsSaving orders writers in this process; the file lock below orders
// writers across a handover. The lock file is never the descriptions file.
var sightsSaving sync.Mutex

// sightsChecked is the last file validated under sightsSaving and its file
// lock. Normal appends only stat the unchanged file; a different writer's
// changes are validated before another append can modify them.
var sightsChecked struct {
	path  string
	info  os.FileInfo
	lines int
}

// savedSight is one completed description in vision-cache.jsonl. Version is
// the on-disk format and the description contract. Kind is explicit so a URL
// can never be revived as a content-addressed image by a later compaction.
type savedSight struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Key     string `json:"key"`
	Text    string `json:"text"`
	By      string `json:"by"`
}

func sightsPath() string { return filepath.Join(settings.Dir(), "vision-cache.jsonl") }

func validSavedSight(v savedSight) bool {
	key, err := hex.DecodeString(v.Key)
	return v.Version == sightCacheVersion && v.Kind == sightKindInline && err == nil && len(key) == 32 &&
		hex.EncodeToString(key) == v.Key && strings.TrimSpace(v.Text) != "" && strings.TrimSpace(v.By) != ""
}

// readSavedSights reads the valid records, newest occurrence winning a
// duplicate key, and returns them oldest to newest with the LRU cap applied.
// A malformed line is reported separately from an invalid record: loading can
// skip it, but compaction must not treat an unreadable file as an empty one.
func readSavedSights(path string) (records []savedSight, lines int, malformed bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, false, err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil {
		return nil, 0, false, err
	} else if info.Size() > sightsMaxDiskBytes {
		return nil, 0, true, fmt.Errorf("vision cache exceeds %d bytes", sightsMaxDiskBytes)
	}
	b, err := io.ReadAll(io.LimitReader(f, sightsMaxDiskBytes+1))
	if err != nil {
		return nil, 0, false, err
	}
	if len(b) > sightsMaxDiskBytes {
		return nil, 0, true, fmt.Errorf("vision cache exceeds %d bytes", sightsMaxDiskBytes)
	}
	latest := map[string]savedSight{}
	last := map[string]int{}
	seq := 0
	for len(b) > 0 {
		var line []byte
		line, b, _ = bytes.Cut(b, []byte{'\n'})
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines++
		var v savedSight
		if json.Unmarshal(line, &v) != nil {
			malformed = true
			continue
		}
		if !validSavedSight(v) {
			continue
		}
		seq++
		latest[v.Key] = v
		last[v.Key] = seq
	}
	for _, v := range latest {
		records = append(records, v)
	}
	sort.Slice(records, func(i, j int) bool { return last[records[i].Key] < last[records[j].Key] })
	if len(records) > sightsKept {
		records = records[len(records)-sightsKept:]
	}
	return records, lines, malformed, nil
}

// loadSights restores completed inline descriptions. Each line is independent:
// a crash-truncated tail doesn't hide the records before it. New doesn't write
// or compact, so a starting gateway can't erase another gateway's new append.
func (s *Server) loadSights() {
	records, lines, malformed, err := readSavedSights(s.sightFile)
	if malformed {
		s.stopSightWrites()
	}
	if err != nil {
		return
	}
	if !malformed {
		s.sightDiskLines = lines
	}
	_ = os.Chmod(s.sightFile, 0o600)
	for _, v := range records {
		done := make(chan struct{})
		close(done)
		s.keepSightLocked(v.Key, &sight{done: done, text: v.Text, by: v.By, answered: true, persistent: true})
	}
}

// beginSightSave counts an inline description that may still have to be
// written: from its creation, before the model is called, until see has
// saved it or failed. Counting from creation closes the window where the
// model has answered but see hasn't published answered yet, so a canceled
// request may return before its save but shutdown still waits for it.
func (s *Server) beginSightSave() {
	s.sightSaveMu.Lock()
	if s.sightSaving == 0 {
		s.sightSavesIdle = make(chan struct{})
	}
	s.sightSaving++
	s.sightSaveMu.Unlock()
}

func (s *Server) endSightSave() {
	s.sightSaveMu.Lock()
	s.sightSaving--
	if s.sightSaving == 0 && s.sightSavesIdle != nil {
		close(s.sightSavesIdle)
	}
	s.sightSaveMu.Unlock()
}

// waitSightSaves waits until no inline description can still save, or ctx
// ends. A nil channel means none has begun since the last one ended.
func (s *Server) waitSightSaves(ctx context.Context) bool {
	s.sightSaveMu.Lock()
	idle := s.sightSavesIdle
	s.sightSaveMu.Unlock()
	if idle == nil {
		return true
	}
	select {
	case <-idle:
		return true
	case <-ctx.Done():
		return false
	}
}

// saveSight appends a completed description before its waiters are released.
// A lock or disk failure only loses persistence, never the successful answer.
// Compaction runs after the append and rereads the latest file under the same
// cross-process lock, so another Server's completed record can't be overwritten.
func (s *Server) saveSight(key, text, by string) {
	v := savedSight{Version: sightCacheVersion, Kind: sightKindInline, Key: key, Text: text, By: by}
	if !validSavedSight(v) {
		return
	}
	line, err := json.Marshal(v)
	if err != nil {
		return
	}
	sightsSaving.Lock()
	defer sightsSaving.Unlock()
	if s.sightDiskLines == sightsDiskDisabled {
		return
	}
	unlock, held := filelock.Lock(s.sightFile+".lock", filelock.Wait)
	defer unlock()
	if !held {
		return
	}
	if !s.canWriteSightsLocked() {
		return
	}
	if s.appendSightLocked(line) != nil {
		sightsChecked.info = nil
		return
	}
	s.sightDiskLines++
	if s.sightDiskLines >= sightsCompactAfter {
		s.compactSightsLocked()
	}
	if s.sightDiskLines == sightsDiskDisabled {
		sightsChecked.info = nil
		return
	}
	sightsChecked.path = s.sightFile
	sightsChecked.info, _ = os.Stat(s.sightFile)
	sightsChecked.lines = s.sightDiskLines
}

// stopSightWrites leaves the original bytes intact and keeps the cache in
// memory only until an operator repairs the file and restarts the gateway.
func (s *Server) stopSightWrites() {
	if s.sightDiskLines != sightsDiskDisabled {
		log.Printf("vision cache: persistence disabled for %s; file left unchanged; repair or move it explicitly and restart the gateway (image descriptions remain cached in memory)", s.sightFile)
	}
	s.sightDiskLines = sightsDiskDisabled
}

func (s *Server) canWriteSightsLocked() bool {
	info, err := os.Stat(s.sightFile)
	if errors.Is(err, os.ErrNotExist) {
		s.sightDiskLines = 0
		return true
	}
	if err != nil {
		return false
	}
	last := sightsChecked.info
	if s.sightFile == sightsChecked.path && last != nil && os.SameFile(info, last) &&
		info.Size() == last.Size() && info.ModTime() == last.ModTime() {
		s.sightDiskLines = max(s.sightDiskLines, sightsChecked.lines)
		return true
	}
	_, lines, malformed, err := readSavedSights(s.sightFile)
	if malformed {
		s.stopSightWrites()
		return false
	}
	if err != nil {
		return false
	}
	s.sightDiskLines = max(s.sightDiskLines, lines)
	sightsChecked.path, sightsChecked.info, sightsChecked.lines = s.sightFile, info, lines
	return true
}

func (s *Server) appendSightLocked(line []byte) error {
	path := s.sightFile
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	// Another writer, or a crash after a partial write, may have changed the
	// tail since this Server loaded it. Check the actual last byte every time.
	if st, err := f.Stat(); err != nil {
		return err
	} else if st.Size() > 0 {
		last := []byte{0}
		if _, err := f.ReadAt(last, st.Size()-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			line = append([]byte{'\n'}, line...)
		}
	}
	line = append(line, '\n')
	_, err = f.Write(line)
	return err
}

// compactSightsLocked rewrites the newest valid records already on disk, not
// this Server's in-memory snapshot. sightsSaving and the file lock are held;
// no sightMu is needed, and URL or in-flight memory entries can't leak here.
func (s *Server) compactSightsLocked() {
	path := s.sightFile
	records, _, malformed, err := readSavedSights(path)
	if malformed {
		s.stopSightWrites()
		return
	}
	if err != nil {
		return
	}
	var out bytes.Buffer
	for _, v := range records {
		line, err := json.Marshal(v)
		if err != nil {
			return
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	target, err := edit.Target(path)
	if err != nil {
		return
	}
	if steady.WriteFile(target, out.Bytes(), 0o600) != nil {
		return
	}
	s.sightDiskLines = len(records)
}
