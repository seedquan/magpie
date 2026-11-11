package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/yetone/magpie/internal/settings"
)

// keepBackups is how many changes' backups are kept.
const keepBackups = 30

// BackupDir contains legacy snapshots and unique skill originals moved aside
// to avoid deleting them. Existing contents are never pruned.
func BackupDir() string { return filepath.Join(settings.Dir(), "backups") }

// backups retains the old result shape without making configuration copies.
type backups struct {
	dir  string
	done map[string]bool
}

func newBackups() *backups {
	return &backups{done: map[string]bool{}}
}

// backupNow is the clock a backup folder is named by.
var backupNow = time.Now

// freshBackup makes a change's backup folder, one no other change has:
// named by the millisecond, with -2, -3… after it when another change
// took that millisecond's. Two changes in one millisecond (a fast
// machine's loop) would otherwise share it, the second's file kept
// under its folder's name beside the first's, and the copy at the name
// the change wrote would be the other change's.
func freshBackup() (string, error) {
	if err := os.MkdirAll(BackupDir(), 0o700); err != nil {
		return "", err
	}
	stamp := backupNow().Format("2006-01-02_15-04-05.000")
	dir := filepath.Join(BackupDir(), stamp)
	for i := 2; ; i++ {
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		dir = filepath.Join(BackupDir(), fmt.Sprintf("%s-%d", stamp, i))
	}
}

// keep validates that an existing file can be read before it is changed,
// without creating a configuration snapshot.
func (b *backups) keep(agent, path string) error {
	_, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// pruneBackups is retained for old callers; removing snapshots does not
// authorize deleting any previously saved user data.
func pruneBackups() {}

// holdsSkills is whether a backup has skill folders in it: <when>/skills,
// or <when>/<agent>/skills.
func holdsSkills(dir string) bool {
	for _, pat := range []string{"skills", "*/skills"} {
		if m, _ := filepath.Glob(filepath.Join(dir, pat)); len(m) > 0 {
			return true
		}
	}
	return false
}
