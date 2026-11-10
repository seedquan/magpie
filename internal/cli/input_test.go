package cli

import (
	"os"
	"strings"
	"testing"
)

func piped(t *testing.T, lines ...string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) > 0 {
		w.WriteString(strings.Join(lines, "\n") + "\n")
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		r.Close()
	})
}
