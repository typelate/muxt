package load

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
}

func packageIDs(pl []*packages.Package) []string {
	var ids []string
	for _, pkg := range pl {
		ids = append(ids, pkg.ID)
	}
	return ids
}

func TestTestVariantsFirst(t *testing.T) {
	in := []*packages.Package{
		{ID: "example.com/a"},
		{ID: "example.com/b [example.com/b.test]"},
		{ID: "example.com/c"},
		{ID: "example.com/a [example.com/a.test]"},
		{ID: "example.com/b.test"},
	}
	want := []string{
		"example.com/b [example.com/b.test]",
		"example.com/a [example.com/a.test]",
		"example.com/a",
		"example.com/c",
		"example.com/b.test",
	}

	assert.Equal(t, want, packageIDs(testVariantsFirst(in)), "testVariantsFirst()")
	assert.Equal(t, "example.com/a", packageIDs(in)[0], "testVariantsFirst reordered its argument")
}

func TestPackagesWithTests(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":    "module example.com/p\n\ngo 1.24\n",
		"p.go":      "package p\n\nfunc F() int { return 1 }\n",
		"p_test.go": "package p\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) { _ = F() }\n",
	})

	pl, err := PackagesWithTests(dir, nil)
	require.NoError(t, err, "PackagesWithTests()")
	require.NotEmpty(t, pl, "PackagesWithTests()")
	require.True(t, strings.HasSuffix(pl[0].ID, ".test]"), "PackagesWithTests()[0] = %v, want the package compiled with its tests first", pl)

	t.Run("the first package holds the test files", func(t *testing.T) {
		var files []string
		for _, f := range pl[0].GoFiles {
			files = append(files, filepath.Base(f))
		}
		assert.Contains(t, files, "p_test.go", "first package files")
	})

	t.Run("the directory resolves to the test variant", func(t *testing.T) {
		got, ok := PackageInDirectory(pl, dir)
		assert.True(t, ok, "PackageInDirectory() ok")
		assert.Same(t, pl[0], got, "PackageInDirectory() want the test variant")
	})
}

func loadError(kind packages.ErrorKind, pos, msg string) packages.Error {
	return packages.Error{Pos: pos, Msg: msg, Kind: kind}
}

// TestParseErrors states what a caller is warned about: source the parser
// could not read, and nothing else.
//
// A type error is how a package looks before muxt generate has written its
// handlers -- a main.go calling a TemplateRoutes that does not exist yet --
// so warning about those would fire on an ordinary first run.
func TestParseErrors(t *testing.T) {
	broken := loadError(packages.ParseError, "server.go:4:1", "expected declaration, found 'return'")

	for _, tt := range []struct {
		name string
		pl   []*packages.Package
		want []string
	}{
		{
			name: "a package that loaded cleanly",
			pl:   []*packages.Package{{PkgPath: "server"}},
		},
		{
			name: "source the parser could not read",
			pl:   []*packages.Package{{PkgPath: "server", Errors: []packages.Error{broken}}},
			want: []string{"server.go:4:1: expected declaration, found 'return'"},
		},
		{
			name: "a type error, which generate has not fixed yet",
			pl: []*packages.Package{{PkgPath: "server", Errors: []packages.Error{
				loadError(packages.TypeError, "main.go:9:2", "undefined: TemplateRoutes"),
			}}},
		},
		{
			name: "a package the loader could not list",
			pl: []*packages.Package{{PkgPath: "server", Errors: []packages.Error{
				loadError(packages.ListError, "", "no Go files"),
			}}},
		},
		{
			name: "one broken file, read by two packages",
			pl: []*packages.Package{
				{PkgPath: "server", Errors: []packages.Error{broken}},
				{PkgPath: "server.test", Errors: []packages.Error{broken}},
			},
			want: []string{"server.go:4:1: expected declaration, found 'return'"},
		},
		{
			name: "two broken files",
			pl: []*packages.Package{{PkgPath: "server", Errors: []packages.Error{
				broken,
				loadError(packages.ParseError, "routes.go:1:1", "expected 'package', found 'func'"),
			}}},
			want: []string{
				"server.go:4:1: expected declaration, found 'return'",
				"routes.go:1:1: expected 'package', found 'func'",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, e := range ParseErrors(tt.pl) {
				got = append(got, e.Error())
			}
			assert.Equal(t, tt.want, got, "ParseErrors")
		})
	}
}

func TestPackagesWithEnvLoadsExtraPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/p\n\ngo 1.24\n",
		"p.go":   "package p\n",
	})
	_, pl, err := PackagesWithEnv(dir, nil, "", "os")
	require.NoError(t, err, "PackagesWithEnv()")
	var paths []string
	for _, pkg := range pl {
		paths = append(paths, pkg.PkgPath)
	}
	for _, want := range []string{"example.com/p", "fmt", "os"} {
		assert.Contains(t, paths, want, "PackagesWithEnv() loaded %q, want it to include %q", paths, want)
	}
}

func TestLoadFailedError(t *testing.T) {
	t.Setenv("GOWORK", "off")
	for _, tt := range []struct {
		name string
		msg  string
		want string
	}{
		{name: "driver plumbing is stripped", msg: "err: exit status 1: stderr: go: boom\n", want: "go: boom"},
		{name: "stderr at the start", msg: "stderr: go: boom", want: "go: boom"},
		{name: "no plumbing", msg: "boom", want: "boom"},
		{
			name: "a go message mentioning stderr is kept whole",
			msg:  "err: exit status 1: stderr: go: reading go.work: stderr: is not a module\n",
			want: "go: reading go.work: stderr: is not a module",
		},
		{name: "only the leading plumbing is plumbing", msg: "go: x stderr: y", want: "go: x stderr: y"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			e, ok := loadFailedError(dir, nil, errors.New(tt.msg)).(*PackageLookupError)
			require.True(t, ok, "loadFailedError() is not a *PackageLookupError")
			want := "failed to load Go packages from " + dir + ": " + tt.want
			assert.Equal(t, want, e.Error(), "loadFailedError(%q).Error() carries the go message", tt.msg)
			require.Len(t, e.Details, 1, "loadFailedError() details = %q, want only the environment note", e.Details)
			assert.Contains(t, e.Details[0], "inherits GOWORK", "loadFailedError() details")
		})
	}
}

// TestLoadFailedErrorWorkspaceEnv states that the workspace hint is about
// the environment the load ran in, not the process's: the mutation run
// loads its --diff copy with GOWORK=off.
func TestLoadFailedErrorWorkspaceEnv(t *testing.T) {
	t.Setenv("GOWORK", "")
	parent := t.TempDir()
	writeFiles(t, parent, map[string]string{"go.work": "go 1.24\n"})
	dir := filepath.Join(parent, "app")
	require.NoError(t, os.Mkdir(dir, 0o700))
	boom := errors.New("err: exit status 1: stderr: go: boom")

	inProcess := loadFailedError(dir, nil, boom).(*PackageLookupError).MultiLineError()
	assert.Contains(t, inProcess, filepath.Base(parent), "loadFailedError(nil env) names the discovered go.work")
	assert.Contains(t, inProcess, "go work use", "loadFailedError(nil env)")

	off := loadFailedError(dir, append(os.Environ(), "GOWORK=off"), boom).(*PackageLookupError).MultiLineError()
	assert.NotContains(t, off, "go work use", "loadFailedError(GOWORK=off env) names a go.work that is not in effect")
}
