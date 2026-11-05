package overlay_test

import (
	"errors"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/overlay"
)

// TestModuleVersion states which github.com/typelate/muxt version the
// pre-pass requires: the one muxt was built as.
func TestModuleVersion(t *testing.T) {
	const modified = "muxt is a modified development build; set MUXT_MODULE_DIR to its source directory"
	devel := func(settings ...string) *debug.BuildInfo {
		info := &debug.BuildInfo{Main: debug.Module{Path: "github.com/typelate/muxt", Version: "(devel)"}}
		for i := 0; i < len(settings); i += 2 {
			info.Settings = append(info.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
		}
		return info
	}
	for _, tt := range []struct {
		name    string
		info    *debug.BuildInfo
		ok      bool
		want    string
		wantErr string
	}{
		{
			name: "a release",
			info: &debug.BuildInfo{Main: debug.Module{Path: "github.com/typelate/muxt", Version: "v0.19.0"}},
			ok:   true, want: "v0.19.0",
		},
		{
			name: "a pseudo-version the go command stamped",
			info: &debug.BuildInfo{Main: debug.Module{Path: "github.com/typelate/muxt", Version: "v0.19.1-0.20261104010203-0123456789ab"}},
			ok:   true, want: "v0.19.1-0.20261104010203-0123456789ab",
		},
		{
			name: "a stamped build with uncommitted changes",
			info: &debug.BuildInfo{Main: debug.Module{Path: "github.com/typelate/muxt", Version: "v0.19.1-0.20261104010203-0123456789ab+dirty"}},
			ok:   true, wantErr: modified,
		},
		{
			name: "a development build of a commit",
			info: devel("vcs", "git", "vcs.revision", "0123456789abcdef0123456789abcdef01234567", "vcs.time", "2026-11-04T01:02:03Z", "vcs.modified", "false"),
			ok:   true, want: "v0.0.0-20261104010203-0123456789ab",
		},
		{
			name: "a development build of a commit in another time zone",
			info: devel("vcs.revision", "0123456789abcdef0123456789abcdef01234567", "vcs.time", "2026-11-04T09:02:03+08:00", "vcs.modified", "false"),
			ok:   true, want: "v0.0.0-20261104010203-0123456789ab",
		},
		{
			name: "a modified development build",
			info: devel("vcs.revision", "0123456789abcdef0123456789abcdef01234567", "vcs.time", "2026-11-04T01:02:03Z", "vcs.modified", "true"),
			ok:   true, wantErr: modified,
		},
		{
			name: "a development build without version control",
			info: devel(),
			ok:   true, wantErr: "set MUXT_MODULE_DIR to its source directory",
		},
		{
			name: "a tool dependency of another module",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/app", Version: "(devel)"},
				Deps: []*debug.Module{{Path: "golang.org/x/net", Version: "v0.62.0"}, {Path: "github.com/typelate/muxt", Version: "v0.18.2"}},
			},
			ok: true, want: "v0.18.2",
		},
		{
			name:    "no build information",
			wantErr: "set MUXT_MODULE_DIR to its source directory",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := overlay.ModuleVersion(tt.info, tt.ok)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("moduleVersion = %q, %v; want an error saying %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("moduleVersion = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// TestProbeRefusalNamesAnUnfetchableModule states that a go list that
// cannot get muxt at the required version says so, not that the overlay
// is refused.
func TestProbeRefusalNamesAnUnfetchableModule(t *testing.T) {
	reason := errors.New("go: github.com/typelate/muxt@v0.19.0: module lookup disabled by GOPROXY=off")
	err := overlay.ProbeRefusal(reason, "v0.19.0")
	for _, want := range []string{"cannot get github.com/typelate/muxt@v0.19.0", "module cache", "module lookup disabled by GOPROXY=off"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("probeRefusal = %q, want it to say %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "refuses an overlay") {
		t.Errorf("probeRefusal = %q, want no overlay refusal", err)
	}
}
