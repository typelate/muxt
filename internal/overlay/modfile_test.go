package overlay_test

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"github.com/typelate/muxt/internal/overlay"
)

// TestWriteModFile states what the modfile copy of a test target's go.mod
// says: the target's go.mod, with github.com/typelate/htt required at
// the version muxt was built with, and the target's own replace of it
// kept.
func TestWriteModFile(t *testing.T) {
	const cli = "v0.2.0"
	for _, tt := range []struct {
		name          string
		goMod         string
		req           overlay.Requirement
		wantRequire   string
		wantReplace   string
		targetVersion string
	}{
		{
			name:        "a module without htt",
			goMod:       "module example.com/target\n\ngo 1.27\n\nrequire golang.org/x/net v0.62.0\n",
			req:         overlay.Requirement{Version: cli},
			wantRequire: cli,
		},
		{
			name:        "htt at the CLI's version",
			goMod:       "module example.com/target\n\ngo 1.27\n\nrequire github.com/typelate/htt " + cli + "\n",
			req:         overlay.Requirement{Version: cli},
			wantRequire: cli,
		},
		{
			name:          "htt at a lower version",
			goMod:         "module example.com/target\n\ngo 1.27\n\nrequire github.com/typelate/htt v0.1.0\n",
			req:           overlay.Requirement{Version: cli},
			wantRequire:   cli,
			targetVersion: "v0.1.0",
		},
		{
			name:          "htt at a higher version",
			goMod:         "module example.com/target\n\ngo 1.27\n\nrequire github.com/typelate/htt v0.3.0\n",
			req:           overlay.Requirement{Version: cli},
			wantRequire:   cli,
			targetVersion: "v0.3.0",
		},
		{
			name:          "a replace of htt the target has",
			goMod:         "module example.com/target\n\ngo 1.27\n\nrequire github.com/typelate/htt v0.1.0\n\nreplace github.com/typelate/htt => /src/htt\n",
			req:           overlay.Requirement{Version: cli},
			wantRequire:   cli,
			wantReplace:   "/src/htt",
			targetVersion: "v0.1.0",
		},
		{
			name:        "a directory replace",
			goMod:       "module example.com/target\n\ngo 1.27\n",
			req:         overlay.Requirement{Version: cli, Dir: "/src/htt"},
			wantRequire: cli,
			wantReplace: "/src/htt",
		},
		{
			name:        "a directory replace over the target's own",
			goMod:       "module example.com/target\n\ngo 1.27\n\nrequire github.com/typelate/htt v0.1.0\n\nreplace github.com/typelate/htt => /elsewhere\n",
			req:         overlay.Requirement{Version: cli, Dir: "/src/htt"},
			wantRequire: cli,
			wantReplace: "/src/htt",
		},
		{
			name:        "a workspace module",
			goMod:       "module example.com/target\n\ngo 1.27\n",
			req:         overlay.Requirement{Dir: "/src/htt"},
			wantRequire: module.ZeroPseudoVersion("v0"),
			wantReplace: "/src/htt",
		},
		{
			name:        "a fork",
			goMod:       "module example.com/target\n\ngo 1.27\n",
			req:         overlay.Requirement{Version: cli, Fork: module.Version{Path: "github.com/x/htt", Version: "v1.2.3"}},
			wantRequire: cli,
			wantReplace: "github.com/x/htt",
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
				if r.Mod.Path != "github.com/typelate/htt" && (len(requires[r.Mod.Path]) != 1 || requires[r.Mod.Path][0] != r.Mod.Version) {
					t.Errorf("modfile requires %s at %q, want the target's %s", r.Mod.Path, requires[r.Mod.Path], r.Mod.Version)
				}
			}
			if v := requires["github.com/typelate/htt"]; len(v) != 1 || v[0] != tt.wantRequire {
				t.Errorf("modfile requires github.com/typelate/htt at %q, want %s:\n%s", v, tt.wantRequire, b)
			}
			var replaces []string
			for _, r := range got.Replace {
				if r.Old.Path == "github.com/typelate/htt" {
					replaces = append(replaces, r.New.Path)
				}
			}
			switch {
			case tt.wantReplace == "" && len(replaces) != 0:
				t.Errorf("modfile replaces github.com/typelate/htt with %q, want no replace", replaces)
			case tt.wantReplace != "" && (len(replaces) != 1 || replaces[0] != tt.wantReplace):
				t.Errorf("modfile replaces github.com/typelate/htt with %q, want %s", replaces, tt.wantReplace)
			}
			if b := readFile(t, filepath.Join(target, "go.mod")); b != tt.goMod {
				t.Errorf("the target's go.mod changed:\n%s", b)
			}
		})
	}
}
