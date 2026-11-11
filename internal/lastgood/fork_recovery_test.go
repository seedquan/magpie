package lastgood

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestForkWritesCreateNoRecoveryCopies(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "corrupt"}[bad], func(t *testing.T) {
			p := setup(t)
			old := []byte(`{"value":"original"}`)
			if bad {
				old = []byte(`{"cut`)
			}
			legacy := []byte(`{"legacy":true}`)
			os.WriteFile(p, old, 0o600)
			os.WriteFile(Bak(p), legacy, 0o600)
			os.WriteFile(p+".bad-existing", []byte("old evidence"), 0o600)
			err := Keep(p, JSON, 0o600)
			if bad && !errors.Is(err, ErrUnreadable) {
				t.Fatalf("corrupt write allowed: %v", err)
			}
			if !bad && err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(Bak(p))
			if !bytes.Equal(got, legacy) {
				t.Fatal("existing backup changed")
			}
			got, _ = os.ReadFile(p)
			if !bytes.Equal(got, old) {
				t.Fatal("original changed")
			}
			files, _ := os.ReadDir(filepath.Dir(p))
			if len(files) != 3 {
				t.Fatalf("created recovery files: %v", files)
			}
		})
	}
}
