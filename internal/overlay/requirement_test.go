package overlay_test

import (
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"golang.org/x/mod/module"

	"github.com/typelate/muxt/internal/overlay"
)

// TestRequirementFor states how the pre-pass requires
// github.com/typelate/htt: at the version muxt was built with, replaced
// as muxt's build replaced it.
func TestRequirementFor(t *testing.T) {
	const noVersion = "muxt's build information does not name a version of github.com/typelate/htt; build muxt with go build or go install"
	htt := func(dep debug.Module) *debug.BuildInfo {
		dep.Path = "github.com/typelate/htt"
		return &debug.BuildInfo{
			Main: debug.Module{Path: "github.com/typelate/muxt", Version: "(devel)"},
			Deps: []*debug.Module{{Path: "golang.org/x/net", Version: "v0.62.0"}, &dep},
		}
	}
	// engine is a directory holding htt, as engineFile names a file in it.
	engine := writeTarget(t, map[string]string{
		"go.mod":               "module github.com/typelate/htt\n\ngo 1.27.0\n",
		"template/template.go": "package template\n",
	})
	engineSource := filepath.Join(engine, "template", "template.go")
	notEngine := writeTarget(t, map[string]string{"go.mod": "module example.com/other\n\ngo 1.27.0\n"})

	for _, tt := range []struct {
		name    string
		info    *debug.BuildInfo
		ok      bool
		file    string
		want    overlay.Requirement
		wantErr string // a substring
	}{
		{
			name: "a release dependency",
			info: htt(debug.Module{Version: "v0.2.0"}), ok: true,
			want: overlay.Requirement{Version: "v0.2.0"},
		},
		{
			name: "a development build of muxt",
			info: htt(debug.Module{Version: "v0.0.0-dev.0"}), ok: true,
			want: overlay.Requirement{Version: "v0.0.0-dev.0"},
		},
		{name: "no build information", wantErr: noVersion},
		{
			name: "no htt dependency",
			info: &debug.BuildInfo{Deps: []*debug.Module{{Path: "golang.org/x/net", Version: "v0.62.0"}}},
			ok:   true, wantErr: noVersion,
		},
		{name: "a version that is not semver", info: htt(debug.Module{Version: "tip"}), ok: true, wantErr: noVersion},
		{
			name: "a directory replace, whose directory is not the one written",
			info: htt(debug.Module{Version: "v0.0.0-dev.0", Replace: &debug.Module{Path: "../htt-local", Version: "(devel)"}}), ok: true,
			file: engineSource,
			want: overlay.Requirement{Version: "v0.0.0-dev.0", Dir: engine},
		},
		{
			name: "a workspace module",
			info: htt(debug.Module{Version: "(devel)"}), ok: true,
			file: engineSource,
			want: overlay.Requirement{Dir: engine},
		},
		{
			name: "a directory and -trimpath",
			info: htt(debug.Module{Version: "(devel)"}), ok: true,
			file:    "github.com/typelate/htt/template/template.go",
			wantErr: "muxt was built with github.com/typelate/htt replaced by a directory and -trimpath, which leaves the directory out; rebuild muxt without -trimpath",
		},
		{
			name: "a directory that is not htt now",
			info: htt(debug.Module{Version: "(devel)"}), ok: true,
			file:    filepath.Join(notEngine, "template", "template.go"),
			wantErr: "muxt was built with github.com/typelate/htt replaced by " + notEngine + ", which does not hold it now: its go.mod is not github.com/typelate/htt's; rebuild muxt",
		},
		{
			name: "a directory without a go.mod",
			info: htt(debug.Module{Version: "(devel)"}), ok: true,
			file:    filepath.Join(t.TempDir(), "template", "template.go"),
			wantErr: "which does not hold it now: ",
		},
		{
			name: "a fork",
			info: htt(debug.Module{Version: "v0.0.0-dev.0", Replace: &debug.Module{Path: "github.com/x/htt", Version: "v1.2.3"}}), ok: true,
			file: "github.com/x/htt@v1.2.3/template/template.go",
			want: overlay.Requirement{Version: "v0.0.0-dev.0", Fork: module.Version{Path: "github.com/x/htt", Version: "v1.2.3"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := overlay.RequirementFor(tt.info, tt.ok, tt.file)
			switch {
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("requirementFor = %+v, %v; want an error saying %q", got, err, tt.wantErr)
				}
			case err != nil || got != tt.want:
				t.Errorf("requirementFor = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}
