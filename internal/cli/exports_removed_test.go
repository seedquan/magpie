package cli

import (
	"bytes"
	"testing"
)

func TestUsageRejectsRemovedExportFlags(t *testing.T) {
	for _, args := range [][]string{{"usage", "--csv"}, {"usage", "--csv", "all"}, {"usage", "--account", "example"}} {
		var out bytes.Buffer
		if err := usageTo(&out, args); err == nil {
			t.Errorf("removed export accepted: %v", args)
		}
		if out.Len() != 0 {
			t.Errorf("export wrote output: %v", args)
		}
	}
}
