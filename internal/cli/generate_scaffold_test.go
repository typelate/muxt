package cli

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// runCommand runs a muxt command line in dir, with no configuration runners,
// and returns what it printed to stdout and the error it returned.
func runCommand(t *testing.T, dir string, commandLine string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	err := commands(dir, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &stdout, io.Discard, runners{})
	return stdout.String(), err
}

// TestGenerateConfigWritesEmptyMuxtJSON states that muxt generate config in a
// directory with no muxt.json creates one with an empty packages object.
func TestGenerateConfigWritesEmptyMuxtJSON(t *testing.T) {
	dir := moduleDir(t)

	stdout, err := runCommand(t, dir, "generate config")

	require.NoError(t, err)
	assert.Equal(t, "wrote muxt.json\n", stdout, "stdout")
	got, err := os.ReadFile(filepath.Join(dir, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, "{\n\t\"packages\": {}\n}\n", string(got), "muxt.json")
}

// TestGenerateConfigLeavesExistingMuxtJSON states that muxt generate config
// fails, and writes nothing, when the working directory has a muxt.json.
func TestGenerateConfigLeavesExistingMuxtJSON(t *testing.T) {
	dir := moduleDir(t)
	const existing = "{\n  \"packages\": {\"example.com/m\": {\"output_htmx\": true}}\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), []byte(existing), 0o644))

	_, err := runCommand(t, dir, "generate config")

	require.EqualError(t, err, filepath.Join(dir, "muxt.json")+" already configures this directory; add a package with: muxt generate package NAME")
	got, err := os.ReadFile(filepath.Join(dir, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, existing, string(got), "muxt.json is unchanged")
}

// TestGeneratePackageAddsEntryToMuxtJSON states that muxt generate package
// adds an entry keyed by the new directory's path from the muxt.json's
// directory, keeps the entries already there, and rewrites the file in its
// stable form.
func TestGeneratePackageAddsEntryToMuxtJSON(t *testing.T) {
	const existing = `{"package_defaults": {"use_templates_variable": ["pages"]}, "packages": {"example.com/m/admin": {"output_htmx": true}}}`
	const want = "{\n" +
		"\t\"package_defaults\": {\n" +
		"\t\t\"use_templates_variable\": [\n" +
		"\t\t\t\"pages\"\n" +
		"\t\t]\n" +
		"\t},\n" +
		"\t\"packages\": {\n" +
		"\t\t\"example.com/m/admin\": {\n" +
		"\t\t\t\"output_htmx\": true\n" +
		"\t\t},\n" +
		"\t\t\"example.com/m/workspace\": {\n" +
		"\t\t\t\"use_receiver_type\": \"T\"\n" +
		"\t\t}\n" +
		"\t}\n" +
		"}\n"

	t.Run("in the file's directory", func(t *testing.T) {
		root := moduleDir(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(existing), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(root, "admin"), 0o755))

		_, err := runCommand(t, root, "generate package workspace")

		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
		require.NoError(t, err)
		assert.Equal(t, want, string(got), "muxt.json")
	})

	t.Run("in a sub-directory", func(t *testing.T) {
		root := moduleDir(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(`{"packages": {}}`), 0o644))
		admin := filepath.Join(root, "admin")
		require.NoError(t, os.Mkdir(admin, 0o755))

		_, err := runCommand(t, admin, "generate package workspace")

		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
		require.NoError(t, err)
		assert.Equal(t, "{\n\t\"packages\": {\n\t\t\"example.com/m/admin/workspace\": {\n\t\t\t\"use_receiver_type\": \"T\"\n\t\t}\n\t}\n}\n", string(got), "muxt.json")
	})
}

// TestGenerateConfigDoesNotFollowDanglingMuxtJSONLink states that muxt generate
// config fails, and creates nothing at the link's target, when muxt.json is a
// symbolic link to a file that does not exist.
func TestGenerateConfigDoesNotFollowDanglingMuxtJSONLink(t *testing.T) {
	dir := moduleDir(t)
	target := filepath.Join(dir, "elsewhere.json")
	link := filepath.Join(dir, "muxt.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	_, err := runCommand(t, dir, "generate config")

	require.EqualError(t, err, link+" already exists; nothing was written")
	_, statErr := os.Lstat(target)
	assert.ErrorIs(t, statErr, os.ErrNotExist, "nothing is created at the link's target")
}

// TestGeneratePackageRecordsGivenFlags states that a generate flag given to
// muxt generate package is recorded under its key, and a flag not given is not.
func TestGeneratePackageRecordsGivenFlags(t *testing.T) {
	for _, tt := range []struct {
		name, commandLine string
		want              map[string]string
		wantAbsent        []string
	}{
		{
			name:        "a switch",
			commandLine: "generate package workspace --output-htmx",
			want:        map[string]string{"output_htmx": "true"},
			wantAbsent:  []string{"output_datastar", "output_file"},
		},
		{
			name:        "a deprecated spelling is recorded under the current key",
			commandLine: "generate package workspace --receiver-type=X --routes-func=R",
			want:        map[string]string{"use_receiver_type": `"X"`, "output_routes_func": `"R"`},
			wantAbsent:  []string{"receiver_type", "routes_func"},
		},
		{
			name:        "a byte size is recorded as exact decimal bytes",
			commandLine: "generate package workspace --output-multipart-max-memory=64MB",
			want:        map[string]string{"output_multipart_max_memory": `"64000000"`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := moduleDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(`{"packages": {}}`), 0o644))

			_, err := runCommand(t, root, tt.commandLine)

			require.NoError(t, err)
			var got composite.File
			data, err := os.ReadFile(filepath.Join(root, "muxt.json"))
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(data, &got))
			entry := got.Packages[importPath("workspace")]
			for key, want := range tt.want {
				assert.JSONEq(t, want, string(entry[key]), "%s of packages[workspace]", key)
			}
			for _, key := range tt.wantAbsent {
				assert.NotContains(t, entry, key, "packages[workspace]")
			}
		})
	}
}

// TestGeneratePackageRejectsInvalidEntryAndWritesNothing states that a merged
// entry generate would reject fails the command, and that nothing is written:
// not the directory, not muxt.json.
func TestGeneratePackageRejectsInvalidEntryAndWritesNothing(t *testing.T) {
	root := moduleDir(t)
	const existing = `{"packages": {}}`
	require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(existing), 0o644))

	_, err := runCommand(t, root, "generate package workspace --output-htmx --output-datastar")

	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(root, "workspace"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "workspace directory is not created")
	got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, existing, string(got), "muxt.json is unchanged")
}

// TestGeneratePackageDeclaresReceiverType states that the Go file of the new
// package declares its package name and the receiver type: the given
// --use-receiver-type, else package_defaults, else T. The entry records the
// type unless package_defaults already supplies that same name.
func TestGeneratePackageDeclaresReceiverType(t *testing.T) {
	for _, tt := range []struct {
		name         string
		defaults     string
		commandLine  string
		wantType     string
		wantRecorded string
	}{
		{name: "default is T", defaults: `{}`, commandLine: "generate package workspace", wantType: "T", wantRecorded: "T"},
		{name: "flag", defaults: `{}`, commandLine: "generate package workspace --use-receiver-type Server", wantType: "Server", wantRecorded: "Server"},
		{name: "package_defaults", defaults: `{"package_defaults": {"use_receiver_type": "Admin"}}`, commandLine: "generate package workspace", wantType: "Admin", wantRecorded: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := moduleDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(tt.defaults), 0o644))

			_, err := runCommand(t, root, tt.commandLine)

			require.NoError(t, err)
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "workspace", "workspace.go"), nil, parser.AllErrors)
			require.NoError(t, err, "workspace.go parses")
			assert.Equal(t, "workspace", file.Name.Name, "package name")
			assert.Contains(t, declaredTypeNames(file), tt.wantType, "receiver type")
			var got composite.File
			data, err := os.ReadFile(filepath.Join(root, "muxt.json"))
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(data, &got))
			recorded, ok := got.Packages[importPath("workspace")]["use_receiver_type"]
			if tt.wantRecorded == "" {
				assert.False(t, ok, "use_receiver_type of packages[workspace]")
			} else {
				require.True(t, ok, "use_receiver_type of packages[workspace]")
				assert.JSONEq(t, strconv.Quote(tt.wantRecorded), string(recorded), "use_receiver_type of packages[workspace]")
			}
		})
	}
}

// declaredTypeNames returns the names of the type declarations in file.
func declaredTypeNames(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			names = append(names, spec.(*ast.TypeSpec).Name.Name)
		}
	}
	return names
}

// TestGeneratePackageWritesFilesAndReportsThem states that muxt generate
// package writes the Go file and an empty template file for the package, and
// prints each path it wrote with the muxt.json entry, relative to the working
// directory.
func TestGeneratePackageWritesFilesAndReportsThem(t *testing.T) {
	root := moduleDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(`{"packages": {}}`), 0o644))

	stdout, err := runCommand(t, root, "generate package workspace")

	require.NoError(t, err)
	assert.Equal(t, "wrote workspace/workspace.go\nwrote workspace/workspace.gohtml\nwrote muxt.json: packages[\"example.com/m/workspace\"]\n", stdout, "stdout")
	html, err := os.ReadFile(filepath.Join(root, "workspace", "workspace.gohtml"))
	require.NoError(t, err)
	assert.Empty(t, html, "workspace.gohtml")
	goSource, err := os.ReadFile(filepath.Join(root, "workspace", "workspace.go"))
	require.NoError(t, err)
	assert.Contains(t, string(goSource), "//go:embed *.gohtml", "workspace.go embeds the templates")
}

// TestGeneratePackageRefusesExistingDirectory states that muxt generate package
// fails, and changes nothing, when the directory it would create exists.
func TestGeneratePackageRefusesExistingDirectory(t *testing.T) {
	root := moduleDir(t)
	const existing = `{"packages": {}}`
	require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(existing), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(root, "workspace"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "workspace", "keep.txt"), []byte("keep"), 0o644))

	_, err := runCommand(t, root, "generate package workspace")

	require.EqualError(t, err, "workspace already exists; nothing was written")
	entries, err := os.ReadDir(filepath.Join(root, "workspace"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "workspace holds only keep.txt")
	got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, existing, string(got), "muxt.json is unchanged")
}

// TestGeneratePackageRejectsBadNames states that a package name that is not an
// identifier, is main, or holds a path separator fails with a message saying
// what NAME is, before anything is written.
func TestGeneratePackageRejectsBadNames(t *testing.T) {
	for _, tt := range []struct {
		name, want string
		dir        string
	}{
		{name: "a-b", want: `package name "a-b" must be a Go identifier`, dir: "a-b"},
		{name: "main", want: `package name "main" is the main package; a route package is a library package the main package imports`, dir: "main"},
		{name: "_hidden", want: `package name "_hidden" starts with _, and the go command ignores a file whose name starts with _`, dir: "_hidden"},
		{name: "a/b", want: `package name "a/b" must be one directory name, created in the working directory`, dir: "a"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := moduleDir(t)
			const existing = `{"packages": {}}`
			require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(existing), 0o644))

			_, err := runCommand(t, root, "generate package "+tt.name)

			var nameErr *composite.PackageNameError
			require.ErrorAs(t, err, &nameErr)
			require.EqualError(t, err, tt.want)
			_, statErr := os.Stat(filepath.Join(root, tt.dir))
			assert.ErrorIs(t, statErr, os.ErrNotExist, "no directory is created")
			got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
			require.NoError(t, err)
			assert.Equal(t, existing, string(got), "muxt.json is unchanged")
		})
	}
}

// TestGeneratePackageNeedsMuxtJSON states that muxt generate package with no
// muxt.json between the working directory and the module root fails with the
// command that creates one, and writes nothing.
func TestGeneratePackageNeedsMuxtJSON(t *testing.T) {
	root := moduleDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644))

	_, err := runCommand(t, root, "generate package workspace")

	require.EqualError(t, err, "no muxt.json found at or above "+root+" up to the module root; create one with: muxt generate config")
	_, statErr := os.Stat(filepath.Join(root, "workspace"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "no directory is created")
}

// TestGenerateConfigurationIsAliasOfConfig states that muxt generate
// configuration does what muxt generate config does.
func TestGenerateConfigurationIsAliasOfConfig(t *testing.T) {
	dir := moduleDir(t)

	stdout, err := runCommand(t, dir, "generate configuration")

	require.NoError(t, err)
	assert.Equal(t, "wrote muxt.json\n", stdout, "stdout")
	got, err := os.ReadFile(filepath.Join(dir, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, "{\n\t\"packages\": {}\n}\n", string(got), "muxt.json")
}

// TestGeneratePackageDeclaresTemplatesVariable states that the Go file of the
// new package declares the first templates variable of the resolved entry: the
// given --use-templates-variable, else package_defaults, else templates, so the
// next generate finds the variable the entry names.
func TestGeneratePackageDeclaresTemplatesVariable(t *testing.T) {
	for _, tt := range []struct {
		name        string
		muxtJSON    string
		commandLine string
		want        string
	}{
		{name: "default", muxtJSON: `{}`, commandLine: "generate package widgets", want: "templates"},
		{name: "flag", muxtJSON: `{}`, commandLine: "generate package widgets --use-templates-variable=widgetTemplates", want: "widgetTemplates"},
		{name: "first of several", muxtJSON: `{}`, commandLine: "generate package widgets --use-templates-variable=a,b", want: "a"},
		{name: "package_defaults", muxtJSON: `{"package_defaults": {"use_templates_variable": ["pages"]}}`, commandLine: "generate package widgets", want: "pages"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := moduleDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(tt.muxtJSON), 0o644))

			_, err := runCommand(t, root, tt.commandLine)

			require.NoError(t, err)
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "widgets", "widgets.go"), nil, parser.AllErrors)
			require.NoError(t, err, "widgets.go parses")
			assert.Contains(t, declaredVarNames(file), tt.want, "templates variable")
		})
	}
}

// declaredVarNames returns the names of the package-level variables in file.
func declaredVarNames(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				names = append(names, name.Name)
			}
		}
	}
	return names
}

// TestGeneratePackageRefusesExistingEntry states that muxt generate package
// fails, and writes nothing, when muxt.json already has an entry for the
// package, even though its directory does not exist.
func TestGeneratePackageRefusesExistingEntry(t *testing.T) {
	root := moduleDir(t)
	existing := `{"packages": {"` + importPath("widgets") + `": {"use_templates_variable": ["x"]}}}`
	require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(existing), 0o644))

	_, err := runCommand(t, root, "generate package widgets")

	var exists *composite.ExistsError
	require.ErrorAs(t, err, &exists)
	require.EqualError(t, err, filepath.Join(root, "muxt.json")+`: packages["`+importPath("widgets")+`"] already exists; nothing was written`)
	_, statErr := os.Stat(filepath.Join(root, "widgets"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "no directory is created")
	got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, existing, string(got), "muxt.json is unchanged")
}

// TestGenerateConfigRefusesDirectoryBelowMuxtJSON states that muxt generate
// config fails, and writes nothing, in a directory below a muxt.json, because
// a nearer file would shadow the one that configures it.
func TestGenerateConfigRefusesDirectoryBelowMuxtJSON(t *testing.T) {
	root := moduleDir(t)
	const existing = `{"packages": {"example.com/m/contacts": {"output_htmx": true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(existing), 0o644))
	contacts := filepath.Join(root, "contacts")
	require.NoError(t, os.Mkdir(contacts, 0o755))

	_, err := runCommand(t, contacts, "generate config")

	var configured *composite.AlreadyConfiguredError
	require.ErrorAs(t, err, &configured)
	require.EqualError(t, err, filepath.Join(root, "muxt.json")+" already configures this directory; add a package with: muxt generate package NAME")
	_, statErr := os.Stat(filepath.Join(contacts, "muxt.json"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "no nearer muxt.json is written")
}

// TestGenerateConfigNeedsModule states that muxt generate config fails, and
// writes nothing, where no go.mod is at or above the working directory,
// because every later command rejects such a file.
func TestGenerateConfigNeedsModule(t *testing.T) {
	dir := t.TempDir()

	_, err := runCommand(t, dir, "generate config")

	var noModule *composite.NoModuleError
	require.ErrorAs(t, err, &noModule)
	require.EqualError(t, err, filepath.Join(dir, "muxt.json")+": no go.mod found at or above the file; package keys are import paths")
	_, statErr := os.Stat(filepath.Join(dir, "muxt.json"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "muxt.json is not written")
}
