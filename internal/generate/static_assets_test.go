package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticAssetsPrefix(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{path: "/static", want: "/static/"},
		{path: "/static/", want: "/static/"},
		{path: "/a/b", want: "/a/b/"},
	} {
		if got := StaticAssetsPrefix(tt.path); got != tt.want {
			t.Errorf("StaticAssetsPrefix(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestStaticAssetsVariable(t *testing.T) {
	for _, tt := range []struct{ routesFunction, want string }{
		{routesFunction: "TemplateRoutes", want: "templateRoutesStaticAssets"},
		{routesFunction: "routes", want: "routesStaticAssets"},
		{routesFunction: "AdminRoutes", want: "adminRoutesStaticAssets"},
	} {
		if got := StaticAssetsVariable(RoutesFileConfiguration{RoutesFunction: tt.routesFunction}); got != tt.want {
			t.Errorf("StaticAssetsVariable(RoutesFunction: %q) = %q, want %q", tt.routesFunction, got, tt.want)
		}
	}
}

// TestCheckStaticAssetsDirectory states what the static assets directory
// must be on disk. The checks against the templates' routes are in the
// err_static_assets_* snapshot archives.
func TestCheckStaticAssetsDirectory(t *testing.T) {
	config := RoutesFileConfiguration{StaticAssetsDirectory: "assets", StaticAssetsPath: "/static"}
	for _, tt := range []struct {
		name    string
		setup   func(t *testing.T, dir string)
		wantErr string
	}{
		{name: "a directory holding a file", setup: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "assets", "app.css"))
		}},
		{name: "a file in a subdirectory", setup: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "assets", "css", "app.css"))
		}},
		{name: "no directory", setup: func(*testing.T, string) {}, wantErr: "--input-static-assets-directory assets: no such directory in {dir}"},
		{name: "a symbolic link", setup: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "real", "app.css"))
			if err := os.Symlink("real", filepath.Join(dir, "assets")); err != nil {
				t.Skip("symbolic links:", err)
			}
		}, wantErr: "--input-static-assets-directory assets: is a symbolic link; name the directory itself"},
		{name: "a file", setup: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "assets"))
		}, wantErr: "--input-static-assets-directory assets: not a directory"},
		{name: "only hidden and underscored names", setup: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "assets", ".hidden.css"))
			writeFile(t, filepath.Join(dir, "assets", "_draft.css"))
			writeFile(t, filepath.Join(dir, "assets", ".git", "config"))
			writeFile(t, filepath.Join(dir, "assets", "_old", "app.css"))
		}, wantErr: "--input-static-assets-directory assets: holds no file go:embed would include"},
		{name: "an empty directory", setup: func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, "assets"), 0o700); err != nil {
				t.Fatal(err)
			}
		}, wantErr: "--input-static-assets-directory assets: holds no file go:embed would include"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			err := checkStaticAssetsDirectory(dir, config, nil)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("checkStaticAssetsDirectory() = %v, want nil", err)
				}
				return
			}
			want := strings.ReplaceAll(tt.wantErr, "{dir}", dir)
			if err == nil || err.Error() != want {
				t.Errorf("checkStaticAssetsDirectory() = %v, want %s", err, want)
			}
		})
	}
}

func writeFile(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
