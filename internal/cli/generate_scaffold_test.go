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
	dir := t.TempDir()

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
	dir := t.TempDir()
	const existing = "{\n  \"packages\": {\".\": {\"output_htmx\": true}}\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), []byte(existing), 0o644))

	_, err := runCommand(t, dir, "generate config")

	require.EqualError(t, err, filepath.Join(dir, "muxt.json")+" already exists; nothing was written")
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
	dir := t.TempDir()
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
	root := moduleDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(`{"packages": {}}`), 0o644))

	_, err := runCommand(t, root, "generate package workspace --output-htmx")

	require.NoError(t, err)
	var got composite.File
	data, err := os.ReadFile(filepath.Join(root, "muxt.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &got))
	entry := got.Packages[importPath("workspace")]
	require.NotNil(t, entry.OutputHTMX, "output_htmx of packages[workspace]")
	assert.True(t, *entry.OutputHTMX, "output_htmx of packages[workspace]")
	assert.Nil(t, entry.OutputDatastar, "output_datastar of packages[workspace]")
	assert.Nil(t, entry.OutputFile, "output_file of packages[workspace]")
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
			recorded := got.Packages[importPath("workspace")].UseReceiverType
			if tt.wantRecorded == "" {
				assert.Nil(t, recorded, "use_receiver_type of packages[workspace]")
			} else {
				require.NotNil(t, recorded, "use_receiver_type of packages[workspace]")
				assert.Equal(t, tt.wantRecorded, *recorded, "use_receiver_type of packages[workspace]")
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

// TestGeneratePackageRejectsNameThatIsNotAnIdentifier states that a package
// name with a path separator fails before anything is written.
func TestGeneratePackageRejectsNameThatIsNotAnIdentifier(t *testing.T) {
	root := moduleDir(t)
	const existing = `{"packages": {}}`
	require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(existing), 0o644))

	_, err := runCommand(t, root, "generate package a/b")

	require.EqualError(t, err, `package name "a/b" must be a Go identifier`)
	_, statErr := os.Stat(filepath.Join(root, "a"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "no directory is created")
	got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, existing, string(got), "muxt.json is unchanged")
}

// TestGeneratePackageNeedsMuxtJSON states that muxt generate package with no
// muxt.json between the working directory and the module root fails with the
// command that creates one, and writes nothing.
func TestGeneratePackageNeedsMuxtJSON(t *testing.T) {
	root := moduleDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644))

	_, err := runCommand(t, root, "generate package workspace")

	require.EqualError(t, err, "no muxt.json found; create one with: muxt generate config")
	_, statErr := os.Stat(filepath.Join(root, "workspace"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "no directory is created")
}

// TestGenerateConfigurationIsAliasOfConfig states that muxt generate
// configuration does what muxt generate config does.
func TestGenerateConfigurationIsAliasOfConfig(t *testing.T) {
	dir := t.TempDir()

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
