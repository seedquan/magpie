// Package lastgood reads legacy recovery files but creates no new copies.
// Writes validate the existing file first. A damaged or unreadable file
// stops the write, preserving its bytes for the user to repair explicitly.
package lastgood

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/steady"
)

// Valid is whether b is a good copy of a file's contents.
type Valid func(b []byte) bool

// JSON is a file that holds one JSON value: a file of zero bytes, empty or
// whitespace only, or cut short is not.
func JSON(b []byte) bool {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	return len(bytes.TrimSpace(b)) > 0 && json.Valid(b)
}

// Bak is where path's last good generation is kept.
func Bak(path string) string { return path + ".bak" }

// ErrUnreadable is a file that is there but neither it nor its .bak is
// valid: unknown, never empty.
var ErrUnreadable = errors.New("unreadable")

// UnreadableError says which file, and why it didn't read.
type UnreadableError struct {
	Path string
	Err  error
}

func (e *UnreadableError) Error() string {
	return fmt.Sprintf("%s can't be read (%v) and has no good backup", e.Path, e.Err)
}

func (e *UnreadableError) Unwrap() []error { return []error{ErrUnreadable, e.Err} }

// Read gives path's contents when they are valid. A file there that isn't
// gives its .bak's contents, when that is valid, and the recovery is noted
// (Recovered). A file not there is fs.ErrNotExist, its .bak not looked at:
// a crash leaves a file renamed over as it was or as it became, never
// gone, so one gone was removed on purpose. Anything else is an
// *UnreadableError, or the error that stopped the read.
func Read(path string, valid Valid) ([]byte, error) {
	b, err := steady.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if valid(b) {
		return b, nil
	}
	return Fallback(path, b, valid)
}

// Fallback is Read for a caller that read path itself and found bad (its
// contents, or nil when only its parse said so): the .bak's contents when
// they are valid, noted, else an *UnreadableError.
func Fallback(path string, bad []byte, valid Valid) ([]byte, error) {
	why := describe(bad)
	if b, err := steady.ReadFile(Bak(path)); err == nil && valid(b) {
		note(path, why)
		return b, nil
	}
	return nil, &UnreadableError{Path: path, Err: errors.New(why)}
}

// describe says what is wrong with a bad file, for the log and the GUI.
func describe(b []byte) string {
	switch {
	case b == nil:
		return "it doesn't parse"
	case len(bytes.Trim(b, "\x00")) == 0 && len(b) > 0:
		return strconv.Itoa(len(b)) + " bytes, every one zero"
	case len(bytes.TrimSpace(b)) == 0:
		return "it is empty"
	}
	return "it doesn't parse"
}

// Keep validates path before replacement without writing a recovery copy.
// A missing file may be created; a damaged one must be repaired explicitly.
// perm is retained for compatibility with the state writers.
func Keep(path string, valid Valid, _ fs.FileMode) error {
	b, err := steady.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !valid(b) {
		return fmt.Errorf("%s cannot be replaced (%s); repair it before saving (recovery copies are disabled): %w", path, describe(b), ErrUnreadable)
	}
	return nil
}

// Into is a file that unmarshals into a T: what a reader that takes any
// other file as empty can read. `[]` is valid JSON but not a map, so a
// reader of a map would see nothing in it.
func Into[T any](b []byte) bool {
	var v T
	return json.Unmarshal(b, &v) == nil
}

// KeepUnparsed validates a file whose reader otherwise treats bad data as
// empty. It leaves malformed data in place and refuses the replacement.
func KeepUnparsed(path string, parses Valid) error {
	return Keep(path, func(b []byte) bool { return len(bytes.TrimSpace(b)) == 0 || parses(b) }, 0)
}

// now is time.Now; a test stands in for it.
var now = time.Now

// A Note is one file read back from its last good generation.
type Note struct {
	File string    `json:"file"` // the file's name: logins.json
	Path string    `json:"path"`
	Why  string    `json:"why"` // what was wrong with it
	Bak  time.Time `json:"bak"` // when the .bak was written
	At   time.Time `json:"at"`
}

var (
	notesMu sync.Mutex
	notes   []Note
)

// note records that path was read from its .bak, once per path until
// Forget: a file read hundreds of times over is logged once.
func note(path, why string) {
	notesMu.Lock()
	defer notesMu.Unlock()
	if slices.ContainsFunc(notes, func(n Note) bool { return n.Path == path }) {
		return
	}
	n := Note{File: filepath.Base(path), Path: path, Why: why, At: now()}
	if fi, err := os.Stat(Bak(path)); err == nil {
		n.Bak = fi.ModTime()
	}
	notes = append(notes, n)
	log.Printf("%s can't be read (%s); what it held at %s is used, from %s", n.File, why, n.Bak.Local().Format("2006-01-02 15:04:05"), filepath.Base(Bak(path)))
}

// Noted records a recovery another process made (the plugin host's of
// plugin-auth.json).
func Noted(path, why string) { note(path, why) }

// Recovered are the files read back from their last good generation since
// magpie started, for the GUI to tell the user.
func Recovered() []Note {
	notesMu.Lock()
	defer notesMu.Unlock()
	return slices.Clone(notes)
}

// Reset forgets the notes; for tests.
func Reset() {
	notesMu.Lock()
	notes = nil
	notesMu.Unlock()
}
