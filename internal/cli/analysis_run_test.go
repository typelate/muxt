package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/load"
)

// analyzeWithLoader runs args in dir with the real check and listing runners
// and the go command, counting how often packages are loaded.
func analyzeWithLoader(t *testing.T, dir string, args ...string) (stdout, stderr string, loads int, err error) {
	t.Helper()
	counting := func(root string, dirs, patterns []string) (*load.Loaded, error) {
		loads++
		return load.InDirectories(root, dirs, patterns)
	}
	var out, errOut bytes.Buffer
	err = commands(dir, args, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &out, &errOut, runners{
		check:   checkRunner(counting),
		calls:   callsRunner(counting),
		callers: callersRunner(counting),
	})
	return out.String(), errOut.String(), loads, err
}

// generatedEntryModule is entryModule with the routes of every entry
// generated, so check has the receiver types it reads from them.
func generatedEntryModule(t *testing.T, receivers map[string]string, imports map[string][]string) string {
	t.Helper()
	dir := entryModule(t, receivers, imports)
	_, _, err := generateWithLoader(t, dir)
	require.NoError(t, err, "muxt generate")
	return dir
}

// TestCheckEveryMuxtJSONEntryFromOneLoad states that muxt check with no flags
// in the directory of a muxt.json checks every entry from one package load, a
// key line ahead of each summary, a package after the packages it imports; and
// that over one package the summary stands alone.
func TestCheckEveryMuxtJSONEntryFromOneLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := generatedEntryModule(t, map[string]string{".": "Root", "workspace": "Space"}, map[string][]string{".": {"workspace"}})

	stdout, _, loads, err := analyzeWithLoader(t, dir, "check")

	require.NoError(t, err, "muxt check")
	assert.Equal(t, 1, loads, "package loads")
	assert.Regexp(t, `\A`+regexp.QuoteMeta(testModule+"/workspace")+`\nok: \d+ templates?\n`+regexp.QuoteMeta(testModule)+`\nok: \d+ templates?\n\z`, stdout, "stdout")

	stdout, _, _, err = analyzeWithLoader(t, filepath.Join(dir, "workspace"), "check")
	require.NoError(t, err, "muxt check in workspace")
	assert.Regexp(t, `\Aok: \d+ templates?\n\z`, stdout, "stdout in one package")
}

// TestCheckStopsAtTheFirstFailingMuxtJSONEntry states that muxt check over
// several entries reports the ones before a failure, then fails naming the
// entry, and does not check the ones after it.
func TestCheckStopsAtTheFirstFailingMuxtJSONEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := generatedEntryModule(t, map[string]string{".": "Root", "workspace": "Space"}, map[string][]string{".": {"workspace"}})
	writeMuxtJSON(t, dir, `{"packages": {
		"example.com/m": {"use_receiver_type": "Root", "use_templates_variable": ["missing"]},
		"example.com/m/workspace": {"use_receiver_type": "Space"}
	}}`)

	stdout, _, _, err := analyzeWithLoader(t, dir, "check")

	require.EqualError(t, err, testModule+": fail: variable missing not found in package "+testModule, "muxt check")
	assert.Regexp(t, `\A`+regexp.QuoteMeta(testModule+"/workspace")+`\nok: \d+ templates?\n\z`, stdout, "the entries reported before the failure")
}

// TestCallsListsEveryMuxtJSONEntryUnderItsKey states that the template
// listings with no flags in the directory of a muxt.json list every entry from
// one package load, in text under a key line and with --format json as one
// array whose elements have a Key; and that over one package there is no key
// and the JSON is the object alone.
func TestCallsListsEveryMuxtJSONEntryUnderItsKey(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := generatedEntryModule(t, map[string]string{".": "Root", "workspace": "Space"}, map[string][]string{".": {"workspace"}})
	for _, command := range []string{"list-template-calls", "list-template-callers"} {
		t.Run(command+" text", func(t *testing.T) {
			stdout, _, loads, err := analyzeWithLoader(t, dir, command)

			require.NoError(t, err, "muxt %s", command)
			assert.Equal(t, 1, loads, "package loads")
			lines := strings.Split(stdout, "\n")
			var keys []string
			for _, line := range lines {
				if strings.HasPrefix(line, testModule) && !strings.Contains(line, ":") && !strings.Contains(line, " ") {
					keys = append(keys, line)
				}
			}
			assert.Equal(t, []string{testModule + "/workspace", testModule}, keys, "key lines in\n%s", stdout)

			stdout, _, _, err = analyzeWithLoader(t, filepath.Join(dir, "workspace"), command)
			require.NoError(t, err, "muxt %s in workspace", command)
			assert.NotContains(t, stdout, testModule+"\n", "a key line over one package")
		})
		t.Run(command+" json", func(t *testing.T) {
			stdout, _, _, err := analyzeWithLoader(t, dir, command, "--format", "json")

			require.NoError(t, err, "muxt %s --format json", command)
			var several []struct{ Key string }
			require.NoError(t, json.Unmarshal([]byte(stdout), &several), "output:\n%s", stdout)
			require.Len(t, several, 2, "output:\n%s", stdout)
			assert.Equal(t, []string{testModule + "/workspace", testModule}, []string{several[0].Key, several[1].Key}, "keys")

			stdout, _, _, err = analyzeWithLoader(t, filepath.Join(dir, "workspace"), command, "--format", "json")
			require.NoError(t, err, "muxt %s --format json in workspace", command)
			var one struct{ Key string }
			require.NoError(t, json.Unmarshal([]byte(stdout), &one), "output:\n%s", stdout)
			assert.Empty(t, one.Key, "key over one package")
		})
	}
}

// TestListingsStopAtTheFirstFailingMuxtJSONEntry states that a listing over
// several entries fails naming the entry that has no templates variable, with
// no "fail: " the way check says it.
func TestListingsStopAtTheFirstFailingMuxtJSONEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{"workspace": "Space"}, nil)
	writeMuxtJSON(t, dir, `{"packages": {
		"example.com/m/workspace": {"use_receiver_type": "Space"},
		"example.com/m/other": {"use_receiver_type": "Other"}
	}}`)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "other"), 0o755))
	writeTestFile(t, filepath.Join(dir, "other"), "other.go", "package other\n")

	_, _, _, err := analyzeWithLoader(t, dir, "list-template-calls")

	require.EqualError(t, err, testModule+"/other: variable templates not found in package "+testModule+"/other", "muxt list-template-calls")
}

// TestCheckNamesTheMuxtJSONEntryAheadOfItsDiagnostics states that muxt check
// over several entries prints the key of the entry that fails ahead of that
// entry's diagnostics, on the stream the diagnostics use, and prints nothing
// for the entries that pass.
func TestCheckNamesTheMuxtJSONEntryAheadOfItsDiagnostics(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := generatedEntryModule(t, map[string]string{".": "Root", "workspace": "Space"}, map[string][]string{".": {"workspace"}})
	writeTestFile(t, dir, "page.gohtml", `{{define "GET /Root RootPage()"}}<p>{{.Missing}}</p>{{end}}`)

	stdout, stderr, _, err := analyzeWithLoader(t, dir, "check")

	require.Error(t, err, "muxt check")
	assert.Regexp(t, `\A`+regexp.QuoteMeta(testModule+"/workspace")+`\nok: \d+ templates?\n\z`, stdout, "stdout")
	assert.Equal(t, testModule+"\n"+
		"$DIR/template_routes.go:38:13 ExecuteTemplate \"GET /Root RootPage()\" *TemplateData[RoutesReceiver, any]\n"+
		"$DIR/page.gohtml:1:39: executing \"GET /Root RootPage()\" at <.Missing>: field or method Missing not found on TemplateData[RoutesReceiver, any]\n"+
		"\n"+
		"Error: "+testModule+": 1 error\n", strings.ReplaceAll(stderr, dir, "$DIR"), "stderr")
}
