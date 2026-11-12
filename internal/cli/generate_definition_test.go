package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestGenerateDefRequiresTemplateFileInWorkingDirectory states that
// --output-file is required and must name a .gohtml file in the working
// directory, and that nothing is written when it does not.
func TestGenerateDefRequiresTemplateFileInWorkingDirectory(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "missing", args: []string{"generate", "def", "GET /foo Index()"}},
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
