package overlay_test

import (
	"runtime/debug"
	"testing"

	"github.com/typelate/muxt/internal/overlay"
)

// TestRequirementFor states which github.com/typelate/htt version the
// pre-pass requires: the one muxt was built with.
func TestRequirementFor(t *testing.T) {
	const noVersion = "muxt's build information does not name a version of github.com/typelate/htt; build muxt with go build or go install"
	htt := func(version string) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main: debug.Module{Path: "github.com/typelate/muxt", Version: "(devel)"},
			Deps: []*debug.Module{{Path: "golang.org/x/net", Version: "v0.62.0"}, {Path: "github.com/typelate/htt", Version: version}},
		}
	}
	for _, tt := range []struct {
		name    string
		info    *debug.BuildInfo
		ok      bool
		want    string
		wantErr string
	}{
		{name: "a release dependency", info: htt("v0.2.0"), ok: true, want: "v0.2.0"},
		{name: "a development build of muxt", info: htt("v0.0.0-dev.0"), ok: true, want: "v0.0.0-dev.0"},
		{name: "no build information", wantErr: noVersion},
		{
			name: "no htt dependency",
			info: &debug.BuildInfo{Deps: []*debug.Module{{Path: "golang.org/x/net", Version: "v0.62.0"}}},
			ok:   true, wantErr: noVersion,
		},
		{name: "htt in a workspace", info: htt("(devel)"), ok: true, wantErr: noVersion},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := overlay.RequirementFor(tt.info, tt.ok, "/src/htt/template/template.go")
			switch {
			case tt.wantErr != "":
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("requirementFor = %+v, %v; want the error %q", got, err, tt.wantErr)
				}
			case err != nil || got.Version != tt.want:
				t.Errorf("requirementFor = %+v, %v; want version %q", got, err, tt.want)
			}
		})
	}
}
