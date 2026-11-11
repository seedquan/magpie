package plugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// plugins#405's literal trailing bytes must remain available for the user to
// repair. Reading the first object cannot justify discarding its tail.
func TestPackageJSONRefusesDamageWithoutCopies(t *testing.T) {
	const whole = "{\n  \"name\": \"magpie-plugins\",\n  \"private\": true,\n  \"dependencies\": {\n    \"@magpie-community/opencode-qoder-auth\": \"0.2.33\",\n    \"@magpie-community/opencode-workbuddy-auth\": \"0.1.29\"\n  }\n}\n"
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"reported tail", whole + "@magpie-community/opencode-zcode-auth\": \"^0.1.27\"\n  }\n}\n", false},
		{"whole", whole, true},
		{"no object", "@magpie\": 1\n}\n", false},
		{"cut short", "{\n  \"name\": \"magpie-plugins\",\n", false},
		{"non object", `[]`, false}, {"null", `null`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storeSandbox(t)
			if err := os.MkdirAll(Dir(), 0o755); err != nil {
				t.Fatal(err)
			}
			pj := filepath.Join(Dir(), "package.json")
			if err := os.WriteFile(pj, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pj+".broken", []byte("older recovery"), 0o600); err != nil {
				t.Fatal(err)
			}
			orig := bunCommand
			bunCommand = func(context.Context, string, string, ...string) *exec.Cmd {
				t.Fatal("Bun ran with a damaged package.json")
				return nil
			}
			t.Cleanup(func() { bunCommand = orig })
			if tc.valid {
				if err := mendPackageJSON(); err != nil {
					t.Fatal(err)
				}
			} else if _, err := runBun(context.Background(), "bun", "remove", "--ignore-scripts", "x"); err == nil || !strings.Contains(err.Error(), "repair it before running Bun") {
				t.Fatalf("invalid manifest was not refused with repair advice: %v", err)
			}
			got, err := os.ReadFile(pj)
			if err != nil || string(got) != tc.body {
				t.Fatalf("original manifest changed: %q, %v", got, err)
			}
			recovery, err := os.ReadFile(pj + ".broken")
			if err != nil || string(recovery) != "older recovery" {
				t.Fatalf("existing recovery changed: %q, %v", recovery, err)
			}
			entries, err := os.ReadDir(Dir())
			if err != nil || len(entries) != 2 {
				t.Fatalf("extra recovery files were created: %v, %v", entries, err)
			}
		})
	}
}

func TestValidPackageJSONAllowsBun(t *testing.T) {
	dir := offSandbox(t)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(Dir(), "package.json"), []byte(`{"name":"magpie-plugins","private":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runBun(context.Background(), "bun", "remove", "--ignore-scripts", "unused"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "bun.log")); err != nil || !strings.Contains(string(b), "remove --ignore-scripts unused") {
		t.Fatalf("valid manifest did not reach Bun: %q, %v", b, err)
	}
}

func TestRemoveRefusesDamagedManifestBeforeChangingPluginList(t *testing.T) {
	storeSandbox(t)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := save(List{Plugins: []Entry{{Spec: "example-plugin@1.2.3"}}}); err != nil {
		t.Fatal(err)
	}
	pj := filepath.Join(Dir(), "package.json")
	const bad = `{"dependencies":{"example-plugin":"1.2.3"}}trailing`
	if err := os.WriteFile(pj, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(context.Background(), "example-plugin"); err == nil || !strings.Contains(err.Error(), "repair it") {
		t.Fatalf("remove hid the damaged manifest: %v", err)
	}
	if l := Load(); len(l.Plugins) != 1 {
		t.Fatalf("failed remove changed the installed list: %+v", l)
	}
	if b, err := os.ReadFile(pj); err != nil || string(b) != bad {
		t.Fatalf("failed remove changed the manifest: %q, %v", b, err)
	}
}
