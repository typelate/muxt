package composite_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// writeIn writes content to name below dir, making the directories on the way.
func writeIn(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// TestFindWalksUpToTheModuleRoot states that Find reads the nearest muxt.json
// at or above the working directory and goes no higher than the directory
// holding go.mod, since a file above the module belongs to another project.
func TestFindWalksUpToTheModuleRoot(t *testing.T) {
	const empty = `{"packages": {}}`
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, root string)
		wd    string
		want  string // directory of the file found, relative to root; "" is none
	}{
		{
			name: "in the working directory",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "a/muxt.json", empty)
			},
			wd:   "a",
			want: "a",
		},
		{
			name: "in a parent",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "muxt.json", empty)
				writeIn(t, root, "a/b/keep", "")
			},
			wd:   "a/b",
			want: ".",
		},
		{
			name: "the nearest wins",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "muxt.json", empty)
				writeIn(t, root, "a/muxt.json", empty)
			},
			wd:   "a",
			want: "a",
		},
		{
			name: "none below the module root",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "a/keep", "")
			},
			wd: "a",
		},
		{
			name: "one above the module root is not read",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "muxt.json", empty)
				writeIn(t, root, "module/go.mod", "module example.com/m\n")
			},
			wd: "module",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)

			f, found, err := composite.Find(filepath.Join(root, tt.wd), testKeys())

			require.NoError(t, err, "Find(%q)", tt.wd)
			if tt.want == "" {
				assert.False(t, found, "Find(%q) found = true, want false", tt.wd)
				return
			}
			require.True(t, found, "Find(%q) found", tt.wd)
			assert.Equal(t, filepath.Join(root, tt.want), f.Dir, "Find(%q) dir", tt.wd)
			assert.Equal(t, filepath.Join(root, tt.want, composite.FileName), f.Name, "Find(%q) name", tt.wd)
		})
	}
}

// TestFindRejectsPackageKeysThatAreNotImportPathsBelowTheFile states that each
// packages key is the clean import path of a package in the file's module, at
// or below the file's directory, and that a key which is not names the file,
// the key and, when there is one, the import path to write.
func TestFindRejectsPackageKeysThatAreNotImportPathsBelowTheFile(t *testing.T) {
	for _, tt := range []struct {
		name, key, dir string
		want           string
		as             func(error) bool
	}{
		{
			name: "dot", key: ".",
			want: `packages key "." is not an import path; write "example.com/m"`,
			as:   isError[*composite.KeyFormError],
		},
		{
			name: "relative directory", key: "workspace",
			want: `packages key "workspace" is not an import path; write "example.com/m/workspace"`,
			as:   isError[*composite.KeyFormError],
		},
		{
			name: "dotted relative directory", key: "./workspace/",
			want: `packages key "./workspace/" is not an import path; write "example.com/m/workspace"`,
			as:   isError[*composite.KeyFormError],
		},
		{
			name: "import path with a trailing slash", key: "example.com/m/workspace/",
			want: `packages key "example.com/m/workspace/" is not an import path; write "example.com/m/workspace"`,
			as:   isError[*composite.KeyFormError],
		},
		{
			name: "parent directory", key: "../other",
			want: `packages key "../other" is outside the file's directory`,
			as:   isError[*composite.KeyOutsideError],
		},
		{
			name: "only the parent directory", key: "..",
			want: `packages key ".." is outside the file's directory`,
			as:   isError[*composite.KeyOutsideError],
		},
		{
			name: "absolute path", key: "/abs",
			want: `packages key "/abs" is outside the file's directory`,
			as:   isError[*composite.KeyOutsideError],
		},
		{
			name: "other module", key: "example.com/other/x",
			want: `packages key "example.com/other/x" is not in this module (example.com/m)`,
			as:   isError[*composite.OtherModuleError],
		},
		{
			name: "module path as a prefix of another", key: "example.com/m2",
			want: `packages key "example.com/m2" is not in this module (example.com/m)`,
			as:   isError[*composite.OtherModuleError],
		},
		{
			name: "in the module, above the file", key: "example.com/m/internal/other", dir: "internal/hypertext",
			want: `packages key "example.com/m/internal/other" is outside the file's directory`,
			as:   isError[*composite.KeyOutsideError],
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeIn(t, root, "go.mod", "module example.com/m\n")
			dir := filepath.Join(root, tt.dir)
			writeIn(t, dir, composite.FileName, `{"packages": {"`+tt.key+`": {}}}`)

			_, _, err := composite.Find(dir, testKeys())

			require.Error(t, err, "Find with key %q", tt.key)
			assert.True(t, tt.as(err), "Find with key %q = %T, want another error type", tt.key, err)
			assert.Equal(t, filepath.Join(dir, composite.FileName)+": "+tt.want, err.Error(), "message")
		})
	}
}

// isError reports whether err matches *T, so a table names the error type a
// row expects without a variable per row.
func isError[T error](err error) bool {
	_, ok := errors.AsType[T](err)
	return ok
}

// TestFindRejectsKeysOfPackageDefaults states that package_defaults holds the
// keys an entry holds: a deprecated or dashed key there is an error that says
// so under package_defaults.
func TestFindRejectsKeysOfPackageDefaults(t *testing.T) {
	keys := testKeys()
	keys.Deprecated = map[string]string{"receiver_type": "use_receiver_type"}
	for _, tt := range []struct {
		name, file, want string
	}{
		{
			name: "deprecated",
			file: `{"package_defaults": {"receiver_type": "T"}, "packages": {"example.com/m": {}}}`,
			want: `package_defaults: key "receiver_type" is deprecated; write "use_receiver_type"`,
		},
		{
			name: "dashed",
			file: `{"package_defaults": {"use-receiver-type": "T"}, "packages": {"example.com/m": {}}}`,
			want: `package_defaults: key "use-receiver-type" uses dashes; write "use_receiver_type"`,
		},
		{
			name: "unknown",
			file: `{"package_defaults": {"zzz": true}, "packages": {"example.com/m": {}}}`,
			want: `package_defaults: unknown key "zzz"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, tt.file)

			_, _, err := composite.Find(dir, keys)

			require.Error(t, err, "Find")
			assert.Equal(t, filepath.Join(dir, composite.FileName)+": "+tt.want, err.Error(), "message")
		})
	}
}

// TestFindRejectsAFileWithoutAModule states that a muxt.json with no go.mod at
// or above it is an error, since its keys are import paths the module path
// names.
func TestFindRejectsAFileWithoutAModule(t *testing.T) {
	dir := t.TempDir()
	writeIn(t, dir, composite.FileName, `{"packages": {"example.com/m": {}}}`)

	_, _, err := composite.Find(dir, testKeys())

	var noModule *composite.NoModuleError
	require.ErrorAs(t, err, &noModule, "Find error %v", err)
	path := filepath.Join(dir, composite.FileName)
	assert.Equal(t, path, noModule.Path, "path")
	assert.Equal(t, path+": no go.mod found at or above the file; package keys are import paths", err.Error(), "message")
}

// TestFindReportsWhereAFileIsNotJSON states that a syntax error is a
// *composite.ParseError at the line and column of the byte the parser
// rejected, in a file found in the working directory or in a parent.
func TestFindReportsWhereAFileIsNotJSON(t *testing.T) {
	for _, tt := range []struct {
		name, file, wd string
		line, col      int
		want           string
	}{
		{
			name: "in the working directory",
			file: "{\n  \"packages\": {\n    \".\": x\n  }\n}\n",
			line: 3, col: 10,
			want: "invalid character 'x' looking for beginning of value",
		},
		{
			name: "in a parent",
			file: "{\n  \"packages\": {\n    \"workspace\": x\n  }\n}\n",
			wd:   "workspace",
			line: 3, col: 18,
			want: "invalid character 'x' looking for beginning of value",
		},
		{
			name: "first line",
			file: "{]",
			line: 1, col: 2,
			want: "invalid character ']' looking for beginning of object key string",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, tt.file)
			wd := filepath.Join(dir, tt.wd)
			require.NoError(t, os.MkdirAll(wd, 0o755))

			_, _, err := composite.Find(wd, testKeys())

			var parse *composite.ParseError
			require.ErrorAs(t, err, &parse, "Find error %v", err)
			path := filepath.Join(dir, composite.FileName)
			assert.Equal(t, path, parse.Path, "path")
			assert.Equal(t, tt.line, parse.Line, "line")
			assert.Equal(t, tt.col, parse.Col, "column")
			assert.Equal(t, fmt.Sprintf("%s:%d:%d: %s", path, tt.line, tt.col, tt.want), err.Error(), "message")
		})
	}
}

// TestKeyForAndDirOfAreInverse states that KeyFor names a directory by its
// import path, the module path joined with its path below the directory
// holding go.mod, wherever the file sits, and DirOf names the directory of an
// import path.
func TestKeyForAndDirOfAreInverse(t *testing.T) {
	root := t.TempDir()
	writeIn(t, root, "go.mod", "module example.com/m\n")
	hypertext := filepath.Join(root, "internal", "hypertext")
	writeIn(t, hypertext, composite.FileName, `{"packages": {}}`)
	f, found, err := composite.Find(hypertext, testKeys())
	require.NoError(t, err, "Find")
	require.True(t, found, "found")

	for _, tt := range []struct {
		dir, key string
	}{
		{dir: root, key: "example.com/m"},
		{dir: hypertext, key: "example.com/m/internal/hypertext"},
		{dir: filepath.Join(hypertext, "workspace"), key: "example.com/m/internal/hypertext/workspace"},
	} {
		got, err := f.KeyFor(tt.dir)
		require.NoError(t, err, "KeyFor(%q)", tt.dir)
		assert.Equal(t, tt.key, got, "KeyFor(%q)", tt.dir)
		assert.Equal(t, tt.dir, f.DirOf(tt.key), "DirOf(%q)", tt.key)
	}
}

// TestFileKeysAreSorted states that Keys returns the packages keys in sorted
// order, however the file listed them.
func TestFileKeysAreSorted(t *testing.T) {
	dir := writeFile(t, `{"packages": {"example.com/m/b": {}, "example.com/m": {}, "example.com/m/a": {}}}`)

	f, _, err := composite.Find(dir, testKeys())

	require.NoError(t, err, "Find")
	assert.Equal(t, []string{"example.com/m", "example.com/m/a", "example.com/m/b"}, f.Keys(), "Keys()")
}

// TestResolveAppliesPackageDefaults states that Resolve gives an entry the
// defaults it leaves out, lets a key the entry holds win, even a false, and
// reports an entry the file does not hold as not found.
func TestResolveAppliesPackageDefaults(t *testing.T) {
	dir := writeFile(t, `{
		"package_defaults": {"use_receiver_type": "T", "output_htmx": true},
		"packages": {
			"example.com/m": {},
			"example.com/m/own": {"use_receiver_type": "Server", "output_htmx": false}
		}
	}`)
	f, _, err := composite.Find(dir, testKeys())
	require.NoError(t, err, "Find")

	for _, tt := range []struct {
		key          string
		receiver     string
		htmx         string
		wantNotFound bool
	}{
		{key: "example.com/m", receiver: `"T"`, htmx: `true`},
		{key: "example.com/m/own", receiver: `"Server"`, htmx: `false`},
		{key: "example.com/m/missing", wantNotFound: true},
	} {
		got, found := f.Resolve(tt.key)

		if tt.wantNotFound {
			assert.False(t, found, "Resolve(%q) found = true, want false", tt.key)
			assert.Empty(t, got, "Resolve(%q)", tt.key)
			continue
		}
		require.True(t, found, "Resolve(%q) found", tt.key)
		assert.JSONEq(t, tt.receiver, string(got["use_receiver_type"]), "Resolve(%q) use_receiver_type", tt.key)
		assert.JSONEq(t, tt.htmx, string(got["output_htmx"]), "Resolve(%q) output_htmx", tt.key)
	}
}

// TestCreateWritesAnEmptyFile states that Create writes a muxt.json with no
// packages that Find then reads.
func TestCreateWritesAnEmptyFile(t *testing.T) {
	root := t.TempDir()
	writeIn(t, root, "go.mod", "module example.com/m\n")

	require.NoError(t, composite.Create(root, testKeys()), "Create")

	got, err := os.ReadFile(filepath.Join(root, composite.FileName))
	require.NoError(t, err, "reading muxt.json")
	assert.Equal(t, "{\n\t\"packages\": {}\n}\n", string(got), "muxt.json")
	f, found, err := composite.Find(root, testKeys())
	require.NoError(t, err, "Find")
	require.True(t, found, "found")
	assert.Empty(t, f.Keys(), "Keys()")
}

// TestCreateRefuses states that Create writes nothing where a muxt.json
// already configures the directory, here or above it, and where no go.mod is
// at or above the directory.
func TestCreateRefuses(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, root string)
		dir   string
		want  func(root string) string
		as    func(error) bool
	}{
		{
			name: "a file in the directory",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, composite.FileName, `{"packages": {}}`)
			},
			want: func(root string) string {
				return filepath.Join(root, composite.FileName) + " already configures this directory; add a package with: muxt generate package NAME"
			},
			as: isError[*composite.AlreadyConfiguredError],
		},
		{
			name: "a file above the directory that would be shadowed",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, composite.FileName, `{"packages": {}}`)
				writeIn(t, root, "a/keep", "")
			},
			dir: "a",
			want: func(root string) string {
				return filepath.Join(root, composite.FileName) + " already configures this directory; add a package with: muxt generate package NAME"
			},
			as: isError[*composite.AlreadyConfiguredError],
		},
		{
			name:  "no go.mod",
			setup: func(*testing.T, string) {},
			want: func(root string) string {
				return root + ": no go.mod found at or above this directory; run go mod init MODULE here first"
			},
			as: isError[*composite.NoModuleError],
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			dir := filepath.Join(root, tt.dir)

			err := composite.Create(dir, testKeys())

			require.Error(t, err, "Create(%q)", dir)
			assert.True(t, tt.as(err), "Create(%q) = %T, want another error type", dir, err)
			assert.Equal(t, tt.want(root), err.Error(), "message")
			if tt.dir != "" {
				_, statErr := os.Stat(filepath.Join(dir, composite.FileName))
				assert.ErrorIs(t, statErr, os.ErrNotExist, "a nearer muxt.json is written")
			}
		})
	}
}
