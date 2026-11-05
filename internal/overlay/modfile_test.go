package overlay_test

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/mod/modfile"

	"github.com/typelate/muxt/internal/overlay"
)

// TestWriteModFile states what the modfile copy of a test target's go.mod
// says: the target's go.mod, with github.com/typelate/muxt required at
// the CLI's version, and replaced only by a module directory the caller
// named.
func TestWriteModFile(t *testing.T) {
	const (
		cli       = "v0.19.0"
		placehold = "v0.0.0-00010101000000-000000000000"
	)
	for _, tt := range []struct {
		name          string
		goMod         string
		req           overlay.Requirement
		wantRequire   string // "" for no requirement of muxt
		wantReplace   string
		targetVersion string
		unchanged     bool
	}{
		{
			name:        "a module without muxt",
			goMod:       "module example.com/target\n\ngo 1.27\n\nrequire golang.org/x/net v0.62.0\n",
			req:         overlay.Requirement{Version: cli},
			wantRequire: cli,
		},
		{
			name:        "a module directory",
			goMod:       "module example.com/target\n\ngo 1.27\n",
			req:         overlay.Requirement{Version: placehold, Dir: "/src/muxt"},
			wantRequire: placehold,
			wantReplace: "/src/muxt",
		},
		{
			name:        "a tool dependency at the CLI's version",
			goMod:       "module example.com/target\n\ngo 1.27\n\ntool github.com/typelate/muxt\n\nrequire github.com/typelate/muxt " + cli + "\n",
			req:         overlay.Requirement{Version: cli},
			wantRequire: cli,
		},
		{
			name:          "a tool dependency at another version",
			goMod:         "module example.com/target\n\ngo 1.27\n\ntool github.com/typelate/muxt\n\nrequire github.com/typelate/muxt v0.18.0\n",
			req:           overlay.Requirement{Version: cli},
			wantRequire:   cli,
			targetVersion: "v0.18.0",
		},
		{
			name:      "muxt itself",
			goMod:     "module github.com/typelate/muxt\n\ngo 1.27\n",
			req:       overlay.Requirement{Version: cli, Dir: "/src/muxt"},
			unchanged: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := writeTarget(t, map[string]string{"go.mod": tt.goMod})
			modFile := filepath.Join(t.TempDir(), "go.mod")
			targetVersion, err := overlay.WriteModFile(filepath.Join(target, "go.mod"), modFile, tt.req)
			if err != nil {
				t.Fatalf("writeModFile: %v", err)
			}
			if targetVersion != tt.targetVersion {
				t.Errorf("the target's own version = %q, want %q", targetVersion, tt.targetVersion)
			}
			b, err := os.ReadFile(modFile)
			if err != nil {
				t.Fatal(err)
			}
			if tt.unchanged {
				if string(b) != tt.goMod {
					t.Errorf("modfile =\n%s\nwant the target's go.mod unchanged:\n%s", b, tt.goMod)
				}
				return
			}
			got, err := modfile.Parse(modFile, b, nil)
			if err != nil {
				t.Fatalf("parsing the modfile: %v\n%s", err, b)
			}
			want, err := modfile.Parse("go.mod", []byte(tt.goMod), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.Module.Mod.Path != want.Module.Mod.Path || got.Go.Version != want.Go.Version {
				t.Errorf("modfile is module %s go %s, want the target's %s go %s", got.Module.Mod.Path, got.Go.Version, want.Module.Mod.Path, want.Go.Version)
			}
			requires := make(map[string][]string)
			for _, r := range got.Require {
				requires[r.Mod.Path] = append(requires[r.Mod.Path], r.Mod.Version)
			}
			for _, r := range want.Require {
				if r.Mod.Path != "github.com/typelate/muxt" && (len(requires[r.Mod.Path]) != 1 || requires[r.Mod.Path][0] != r.Mod.Version) {
					t.Errorf("modfile requires %s at %q, want the target's %s", r.Mod.Path, requires[r.Mod.Path], r.Mod.Version)
				}
			}
			if v := requires["github.com/typelate/muxt"]; len(v) != 1 || v[0] != tt.wantRequire {
				t.Errorf("modfile requires github.com/typelate/muxt at %q, want %s:\n%s", v, tt.wantRequire, b)
			}
			var replaces []string
			for _, r := range got.Replace {
				if r.Old.Path == "github.com/typelate/muxt" {
					replaces = append(replaces, r.New.Path)
				}
			}
			switch {
			case tt.wantReplace == "" && len(replaces) != 0:
				t.Errorf("modfile replaces github.com/typelate/muxt with %q, want no replace", replaces)
			case tt.wantReplace != "" && (len(replaces) != 1 || replaces[0] != tt.wantReplace):
				t.Errorf("modfile replaces github.com/typelate/muxt with %q, want %s", replaces, tt.wantReplace)
			}
			if b := readFile(t, filepath.Join(target, "go.mod")); b != tt.goMod {
				t.Errorf("the target's go.mod changed:\n%s", b)
			}
		})
	}
}
