package gui

import (
	"github.com/yetone/magpie/internal/appdir"
	"os"
	"path/filepath"
)

func downloads() string {
	home, _ := os.UserHomeDir()
	for _, d := range []string{appdir.Downloads(), filepath.Join(home, "Downloads")} {
		if d != "" && isDir(d) {
			return d
		}
	}
	return home
}

func isDir(p string) bool { info, err := os.Stat(p); return err == nil && info.IsDir() }
