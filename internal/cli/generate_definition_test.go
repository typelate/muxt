package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
)

// runCommandArgs runs a muxt command with args in dir, which a template name
// holding spaces needs, and returns what it printed to stdout and stderr and
// the error it returned.
func runCommandArgs(dir string, args []string, run runners) (stdout, stderr string, err error) {
	var out, errOut bytes.Buffer
	err = commands(dir, args, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &out, &errOut, run)
	return out.String(), errOut.String(), err
}

// TestGenerateDefAppendsDefineToTemplateFile states that muxt generate def
// ends the named template file with a define named for the route, creating the
// file when it is missing, starting the define on its own line, and leaving the
// file ending in one newline.
func TestGenerateDefAppendsDefineToTemplateFile(t *testing.T) {
	const define = `{{define "GET /foo Index()"}}{{end}}`
	for _, tt := range []struct {
		name     string
		existing string
		exists   bool
		want     string
	}{
		{name: "missing file", want: define + "\n"},
		{name: "file ends with a newline", exists: true, existing: "{{define \"a\"}}{{end}}\n", want: "{{define \"a\"}}{{end}}\n" + define + "\n"},
		{name: "file does not end with a newline", exists: true, existing: "{{define \"a\"}}{{end}}", want: "{{define \"a\"}}{{end}}\n" + define + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.exists {
				writeTestFile(t, dir, "index.gohtml", tt.existing)
			}

			_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo Index()", "--output-file=index.gohtml"}, runners{})

			require.NoError(t, err)
			got, err := os.ReadFile(filepath.Join(dir, "index.gohtml"))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got), "index.gohtml")
		})
	}
}

// TestGenerateDefRequiresTemplateFileInWorkingDirectory states that a given
// --output-file must name a .gohtml file in the working directory, and that
// nothing is written when it does not.
func TestGenerateDefRequiresTemplateFileInWorkingDirectory(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "wrong extension", args: []string{"generate", "def", "GET /foo Index()", "--output-file=index.html"}},
		{name: "path separator", args: []string{"generate", "def", "GET /foo Index()", "--output-file=sub/index.gohtml"}},
		{name: "no stem", args: []string{"generate", "def", "GET /foo Index()", "--output-file=.gohtml"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			_, _, err := runCommandArgs(dir, tt.args, runners{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "--output-file", "error names the flag")
			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			assert.Empty(t, entries, "nothing is written")
		})
	}
}

// TestGenerateDefWithoutOutputFilePrintsArchive states that without
// --output-file, muxt generate def prints a txtar archive of the template file
// it would write, with the body the body flag chooses, and creates nothing.
func TestGenerateDefWithoutOutputFilePrintsArchive(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "fragment", want: "-- index.gohtml --\n{{define \"GET /foo Index()\"}}{{end}}\n"},
		{name: "document", args: []string{"--output-document"}, want: "-- index.gohtml --\n" + generate.DefineBlock("GET /foo Index()", generate.DocumentBody) + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"generate", "def", "GET /foo Index()"}, tt.args...)

			stdout, stderr, err := runCommandArgs(dir, args, runners{})

			require.NoError(t, err)
			assert.Equal(t, tt.want, stdout, "stdout")
			assert.Empty(t, stderr, "stderr")
			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			assert.Empty(t, entries, "nothing is written")
		})
	}
}

// TestGenerateDefWithoutOutputFilePrintsStubInArchive states that when the
// receiver type lacks the route's method, the archive gains a .go section
// holding the stub, and both sections are named for the route's identifier in
// snake case.
func TestGenerateDefWithoutOutputFilePrintsStubInArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	for _, tt := range []struct {
		name  string
		route string
		base  string
		stub  string
	}{
		{name: "one word method", route: "GET /foo Index()", base: "index", stub: "func (T) Index() any {\n\treturn nil\n}\n"},
		{name: "multi-word method", route: "GET /foo ListArticles()", base: "list_articles", stub: "func (T) ListArticles() any {\n\treturn nil\n}\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := definitionModule(t, "", nil)

			stdout, stderr, err := runCommandArgs(dir, []string{"generate", "def", tt.route, "--use-receiver-type=T"}, runners{load: load.InDirectories})

			require.NoError(t, err)
			want := "-- " + tt.base + ".gohtml --\n" + generate.DefineBlock(tt.route, generate.FragmentBody) + "\n" +
				"-- " + tt.base + ".go --\n" + tt.stub
			assert.Equal(t, want, stdout, "stdout")
			assert.Empty(t, stderr, "stderr")
			archive := txtar.Parse([]byte(stdout))
			var names []string
			for _, f := range archive.Files {
				names = append(names, f.Name)
			}
			assert.Equal(t, []string{tt.base + ".gohtml", tt.base + ".go"}, names, "archive file names")
		})
	}
}

// TestGenerateDefReportsWriteOnStderr states that the status line goes to
// stderr, which leaves stdout for the method stub.
func TestGenerateDefReportsWriteOnStderr(t *testing.T) {
	dir := t.TempDir()

	stdout, stderr, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo Index()", "--output-file=index.gohtml"}, runners{})

	require.NoError(t, err)
	assert.Equal(t, "wrote index.gohtml: GET /foo Index()\n", stderr, "stderr")
	assert.Empty(t, stdout, "stdout")
}

// TestGenerateDefinitionIsAliasOfDef states that muxt generate definition does
// what muxt generate def does.
func TestGenerateDefinitionIsAliasOfDef(t *testing.T) {
	dir := t.TempDir()

	_, stderr, err := runCommandArgs(dir, []string{"generate", "definition", "GET /foo Index()", "--output-file=index.gohtml"}, runners{})

	require.NoError(t, err)
	assert.Equal(t, "wrote index.gohtml: GET /foo Index()\n", stderr, "stderr")
	got, err := os.ReadFile(filepath.Join(dir, "index.gohtml"))
	require.NoError(t, err)
	assert.Equal(t, "{{define \"GET /foo Index()\"}}{{end}}\n", string(got), "index.gohtml")
}

// TestGenerateDefRejectsNameThatIsNotARoute states that a name muxt would not
// accept as a route template fails with the error muxt gives for it, and that
// nothing is written.
func TestGenerateDefRejectsNameThatIsNotARoute(t *testing.T) {
	for _, tt := range []struct {
		name string
		want string
	}{
		{name: "GET foo Index(", want: `"GET foo Index(" is not a route template name`},
		{name: "header", want: `"header" is not a route template name`},
		{name: "GET /foo Index(", want: "failed to parse handler expression: template_name.go:1:7: expected ')', found 'EOF'"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			_, _, err := runCommandArgs(dir, []string{"generate", "def", tt.name, "--output-file=index.gohtml"}, runners{})

			require.EqualError(t, err, tt.want)
			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			assert.Empty(t, entries, "nothing is written")
		})
	}
}

// definitionModule writes a module whose root package parses *.gohtml into
// templates, with methods declared after its type T and files beside it, and
// returns its directory. The package always holds base.gohtml, so the embed
// pattern has a match before the command adds a file.
func definitionModule(t *testing.T, methods string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")
	writeTestFile(t, dir, "base.gohtml", `{{define "base"}}{{end}}`)
	writeTestFile(t, dir, "m.go", `package m

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var source embed.FS

var templates = template.Must(template.ParseFS(source, "*.gohtml"))

type T struct{}
`+methods)
	for name, content := range files {
		writeTestFile(t, dir, name, content)
	}
	return dir
}

// TestGenerateDefRefusesNameAlreadyDefined states that a name the package's
// templates already define fails, naming the file and line relative to the
// working directory, and that nothing is written.
func TestGenerateDefRefusesNameAlreadyDefined(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := definitionModule(t, "", map[string]string{
		"index.gohtml": "{{define \"GET /bar Bar()\"}}{{end}}\n\n{{define \"GET /foo Index()\"}}{{end}}\n",
	})

	_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo Index()", "--output-file=other.gohtml"}, runners{load: load.InDirectories})

	require.EqualError(t, err, `template "GET /foo Index()" is already defined at index.gohtml:3`)
	duplicate, ok := errors.AsType[*generate.DuplicateDefinitionError](err)
	require.True(t, ok, "error is a *generate.DuplicateDefinitionError")
	assert.Equal(t, generate.DuplicateDefinitionError{Name: "GET /foo Index()", File: "index.gohtml", Line: 3}, *duplicate)
	assert.NoFileExists(t, filepath.Join(dir, "other.gohtml"))
}

// TestGenerateDefSaysWhenItCannotCheckForDuplicates states that a package with
// no templates variable fails plainly instead of skipping the check, and that
// nothing is written.
func TestGenerateDefSaysWhenItCannotCheckForDuplicates(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")
	writeTestFile(t, dir, "m.go", "package m\n")

	_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo Index()", "--output-file=index.gohtml"}, runners{load: load.InDirectories})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `cannot check whether "GET /foo Index()" is already defined`)
	assert.NoFileExists(t, filepath.Join(dir, "index.gohtml"))
}

// TestGenerateDefOutputDocumentWritesAHTMLDocument states that
// --output-document makes the define a full HTML document.
func TestGenerateDefOutputDocumentWritesAHTMLDocument(t *testing.T) {
	dir := t.TempDir()

	_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo Index()", "--output-file=index.gohtml", "--output-document"}, runners{})

	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(dir, "index.gohtml"))
	require.NoError(t, err)
	assert.Equal(t, generate.DefineBlock("GET /foo Index()", generate.DocumentBody)+"\n", string(got), "index.gohtml")
	assert.Contains(t, string(got), "<!DOCTYPE html>\n<html lang=\"en\">", "index.gohtml holds a document")
}

// TestGenerateDefOutputFragmentWritesAnEmptyDefine states that
// --output-fragment, or neither of the two body flags, makes the define empty.
func TestGenerateDefOutputFragmentWritesAnEmptyDefine(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "fragment flag", args: []string{"--output-fragment"}},
		{name: "neither flag"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"generate", "def", "GET /foo Index()", "--output-file=index.gohtml"}, tt.args...)

			_, _, err := runCommandArgs(dir, args, runners{})

			require.NoError(t, err)
			got, err := os.ReadFile(filepath.Join(dir, "index.gohtml"))
			require.NoError(t, err)
			assert.Equal(t, "{{define \"GET /foo Index()\"}}{{end}}\n", string(got), "index.gohtml")
		})
	}
}

// TestGenerateDefRejectsFragmentWithDocument states that the two body flags
// are mutually exclusive, and that nothing is written.
func TestGenerateDefRejectsFragmentWithDocument(t *testing.T) {
	dir := t.TempDir()

	_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo Index()", "--output-file=index.gohtml", "--output-fragment", "--output-document"}, runners{})

	require.EqualError(t, err, "--output-fragment and --output-document are mutually exclusive")
	assert.NoFileExists(t, filepath.Join(dir, "index.gohtml"))
}

// TestGenerateDefPrintsMethodStubForUndefinedMethod states that stdout holds a
// method stub, and nothing else, when the receiver type has no method the route
// calls: the receiver comes from the muxt.json entry with package_defaults
// applied, else from --use-receiver-type, and is a pointer receiver when the
// type already has a method on a pointer.
func TestGenerateDefPrintsMethodStubForUndefinedMethod(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	const (
		valueStub   = "func (T) Index(ctx context.Context, id string) any {\n\treturn nil\n}\n"
		pointerStub = "func (*T) Index(ctx context.Context, id string) any {\n\treturn nil\n}\n"
	)
	for _, tt := range []struct {
		name     string
		methods  string
		muxtJSON string
		flags    []string
		want     string
	}{
		{name: "entry names the receiver", muxtJSON: `{"packages": {"example.com/m": {"use_receiver_type": "T"}}}`, want: valueStub},
		{name: "package_defaults names the receiver", muxtJSON: `{"package_defaults": {"use_receiver_type": "T"}, "packages": {"example.com/m": {}}}`, want: valueStub},
		{name: "flag names the receiver in a configured directory", muxtJSON: `{"packages": {"example.com/m": {}}}`, flags: []string{"--use-receiver-type=T"}, want: valueStub},
		{name: "flag names the receiver when only package_defaults is set", muxtJSON: `{"package_defaults": {"use_templates_variable": ["templates"]}, "packages": {"example.com/m": {}}}`, flags: []string{"--use-receiver-type=T"}, want: valueStub},
		{name: "flag names the receiver", flags: []string{"--use-receiver-type=T"}, want: valueStub},
		{name: "type has a pointer method", methods: "\nfunc (*T) Other() any { return nil }\n", flags: []string{"--use-receiver-type=T"}, want: pointerStub},
		{name: "no receiver type known", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{}
			if tt.muxtJSON != "" {
				files["muxt.json"] = tt.muxtJSON
			}
			dir := definitionModule(t, tt.methods, files)
			args := append([]string{"generate", "def", "GET /foo/{id} Index(ctx, id)", "--output-file=index.gohtml"}, tt.flags...)

			stdout, _, err := runCommandArgs(dir, args, runners{load: load.InDirectories})

			require.NoError(t, err)
			assert.Equal(t, tt.want, stdout, "stdout")
		})
	}
}

// TestGenerateDefRejectsReceiverTypeFlagInConfiguredDirectory states that
// --use-receiver-type fails with a *composite.ConfiguredDirectoryError when the
// directory has a muxt.json entry, since the entry already names the receiver.
func TestGenerateDefRejectsReceiverTypeFlagInConfiguredDirectory(t *testing.T) {
	dir := moduleDir(t)
	writeTestFile(t, dir, "muxt.json", `{"packages": {"example.com/m": {"use_receiver_type": "T"}}}`)

	_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo Index()", "--output-file=index.gohtml", "--use-receiver-type=U"}, runners{})

	_, ok := errors.AsType[*composite.ConfiguredDirectoryError](err)
	require.True(t, ok, "error is a *composite.ConfiguredDirectoryError: %v", err)
	assert.Equal(t, "configure generate def with fields in "+filepath.Join(dir, "muxt.json")+" instead of flags", err.Error(), "message")
	assert.NoFileExists(t, filepath.Join(dir, "index.gohtml"))
}

// TestGenerateDefRejectsReceiverTypeFlagWhenPackageDefaultsNamesOne states that
// the flag is refused when only package_defaults names the receiver, since the
// resolved entry names one.
func TestGenerateDefRejectsReceiverTypeFlagWhenPackageDefaultsNamesOne(t *testing.T) {
	dir := moduleDir(t)
	writeTestFile(t, dir, "muxt.json", `{"package_defaults": {"use_receiver_type": "T"}, "packages": {"example.com/m": {}}}`)

	_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo Index()", "--output-file=index.gohtml", "--use-receiver-type=U"}, runners{})

	_, ok := errors.AsType[*composite.ConfiguredDirectoryError](err)
	require.True(t, ok, "error is a *composite.ConfiguredDirectoryError: %v", err)
	assert.Equal(t, "configure generate def with fields in "+filepath.Join(dir, "muxt.json")+" instead of flags", err.Error(), "message")
	assert.NoFileExists(t, filepath.Join(dir, "index.gohtml"))
}

// TestGenerateDefPrintsNoStubForDefinedMethod states that stdout is empty, and
// the define is still written, when the receiver type already has the method.
func TestGenerateDefPrintsNoStubForDefinedMethod(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := definitionModule(t, "\nfunc (T) Index(ctx context.Context, id string) any { return nil }\n", map[string]string{
		"muxt.json": `{"packages": {"example.com/m": {"use_receiver_type": "T"}}}`,
	})
	goFile, err := os.ReadFile(filepath.Join(dir, "m.go"))
	require.NoError(t, err)
	writeTestFile(t, dir, "m.go", strings.Replace(string(goFile), "import (\n", "import (\n\t\"context\"\n", 1))

	stdout, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /foo/{id} Index(ctx, id)", "--output-file=index.gohtml"}, runners{load: load.InDirectories})

	require.NoError(t, err)
	assert.Empty(t, stdout, "stdout")
	got, err := os.ReadFile(filepath.Join(dir, "index.gohtml"))
	require.NoError(t, err)
	assert.Equal(t, "{{define \"GET /foo/{id} Index(ctx, id)\"}}{{end}}\n", string(got), "index.gohtml")
}

// TestGenerateDefRefusesFileTheTemplatesVariableDoesNotRead states that
// --output-file fails, naming the file by absolute path, the variable and the
// patterns, and writes nothing, when the file matches no //go:embed pattern or
// no ParseFS pattern of the templates variable, and that a file both match is
// written.
func TestGenerateDefRefusesFileTheTemplatesVariableDoesNotRead(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	module := func(t *testing.T, embed, parse string) string {
		t.Helper()
		dir := t.TempDir()
		writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")
		writeTestFile(t, dir, "page.gohtml", `{{define "GET /page Page()"}}{{end}}`)
		writeTestFile(t, dir, "m.go", "package m\n\nimport (\n\t\"embed\"\n\t\"html/template\"\n)\n\n//go:embed "+embed+"\nvar source embed.FS\n\nvar templates = template.Must(template.ParseFS(source, "+parse+"))\n\ntype T struct{}\n")
		return dir
	}
	for _, tt := range []struct {
		name, embed, parse, want string
	}{
		{name: "embed pattern misses", embed: "page.gohtml", parse: `"*.gohtml"`, want: `%s is not read by templates: its //go:embed patterns are "page.gohtml" and its ParseFS patterns are "*.gohtml"; name a file both match`},
		{name: "ParseFS pattern misses", embed: "*.gohtml", parse: `"page.gohtml"`, want: `%s is not read by templates: its //go:embed patterns are "*.gohtml" and its ParseFS patterns are "page.gohtml"; name a file both match`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := module(t, tt.embed, tt.parse)

			_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /x X()", "--output-file=extra.gohtml"}, runners{load: load.InDirectories})

			require.EqualError(t, err, fmt.Sprintf(tt.want, filepath.Join(dir, "extra.gohtml")))
			_, ok := errors.AsType[*load.UnreadFileError](err)
			assert.True(t, ok, "error is a *load.UnreadFileError")
			assert.NoFileExists(t, filepath.Join(dir, "extra.gohtml"))
		})
	}
	t.Run("both match", func(t *testing.T) {
		dir := module(t, "*.gohtml", `"*.gohtml"`)

		_, _, err := runCommandArgs(dir, []string{"generate", "def", "GET /x X()", "--output-file=extra.gohtml"}, runners{load: load.InDirectories})

		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(dir, "extra.gohtml"))
	})
}
