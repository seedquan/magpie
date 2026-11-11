package lastgood

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// zeros is what #1505's Windows crash left: logins.json at its full
// length, 10,783 bytes, every one 0x00.
func zeros(n int) []byte { return make([]byte, n) }

func setup(t *testing.T) string {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
	return filepath.Join(t.TempDir(), "logins.json")
}

func TestZeroedFileReadFromItsBackup(t *testing.T) {
	p := setup(t)
	good := []byte(`[{"agent":"codex","user":"a@x.com"}]` + "\n")
	if err := os.WriteFile(p, good, 0o600); err != nil {
		t.Fatal(err)
	}
	// An existing backup remains readable; new writes do not create one.
	os.WriteFile(Bak(p), good, 0o600)
	if err := os.WriteFile(p, zeros(10783), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := Read(p, JSON)
	if err != nil || !bytes.Equal(b, good) {
		t.Fatalf("a zeroed file read as %q, %v; want its backup", b, err)
	}
	n := Recovered()
	if len(n) != 1 || n[0].File != "logins.json" || n[0].Why != "10783 bytes, every one zero" || n[0].Bak.IsZero() {
		t.Fatalf("recovery noted as %+v", n)
	}
	// read again, it is noted once
	Read(p, JSON)
	if len(Recovered()) != 1 {
		t.Fatalf("noted %d times", len(Recovered()))
	}
}

// With no good backup, a zeroed file is unknown, never empty, and a file
// not there is still not there.
func TestZeroedFileWithoutBackupIsUnknown(t *testing.T) {
	p := setup(t)
	if _, err := Read(p, JSON); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing file read as %v", err)
	}
	os.WriteFile(p, zeros(29926), 0o600)
	if b, err := Read(p, JSON); !errors.Is(err, ErrUnreadable) || b != nil {
		t.Fatalf("a zeroed file without a backup read as %q, %v", b, err)
	}
	os.WriteFile(Bak(p), zeros(29926), 0o600) // the backup zeroed too
	if _, err := Read(p, JSON); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("a zeroed backup taken: %v", err)
	}
	if len(Recovered()) != 0 {
		t.Fatalf("noted %+v", Recovered())
	}
}

// A damaged file stops writes without generating or changing recovery copies.
func TestKeepRejectsABadFile(t *testing.T) {
	p := setup(t)
	old := zeros(10783)
	os.WriteFile(p, old, 0o600)
	if err := Keep(p, JSON, 0o600); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("Keep = %v", err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, old) {
		t.Fatal("damaged original changed")
	}
	if files, _ := os.ReadDir(filepath.Dir(p)); len(files) != 1 {
		t.Fatal("new recovery copies created")
	}
}

func TestDescribe(t *testing.T) {
	for b, want := range map[string]string{
		string(zeros(3)): "3 bytes, every one zero",
		"":               "it is empty",
		"  \n":           "it is empty",
		`{"a":`:          "it doesn't parse",
	} {
		if got := describe([]byte(b)); got != want {
			t.Errorf("describe(%q) = %q, want %q", b, got, want)
		}
	}
	if JSON([]byte("\xef\xbb\xbf{}")) != true || JSON(zeros(5)) || JSON(nil) {
		t.Error("JSON misjudged")
	}
}

func TestKeepUnparsed(t *testing.T) {
	p := setup(t)
	parses := Into[map[string]string]
	if err := KeepUnparsed(p, parses); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`{"a":"b"}`, "", "  \n", "null", `[]`, `{"a":"b"`, `{"a":1}`} {
		os.WriteFile(p, []byte(text), 0o600)
		err := KeepUnparsed(p, parses)
		valid := len(bytes.TrimSpace([]byte(text))) == 0 || parses([]byte(text))
		if valid && err != nil || !valid && !errors.Is(err, ErrUnreadable) {
			t.Fatalf("%q: %v", text, err)
		}
		if files, _ := os.ReadDir(filepath.Dir(p)); len(files) != 1 {
			t.Fatal("new recovery copies created")
		}
	}
}
