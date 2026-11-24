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

// TestOneMuxtJSONEntryIsKeyedAsTwoAre states that a flagless run in the
// directory of a muxt.json with one entry prints the entry's key line in text
// and writes the keyed array with --format json, as it does for two entries,
// so a script reading the output does not change when an entry is added.
func TestOneMuxtJSONEntryIsKeyedAsTwoAre(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := generatedEntryModule(t, map[string]string{".": "Root"}, nil)

	stdout, _, _, err := analyzeWithLoader(t, dir, "check")
	require.NoError(t, err, "muxt check")
	assert.Regexp(t, `\A`+regexp.QuoteMeta(testModule)+`\nok: \d+ templates?\n\z`, stdout, "check stdout")

	for _, command := range []string{"list-template-calls", "list-template-callers"} {
		t.Run(command, func(t *testing.T) {
			stdout, _, _, err := analyzeWithLoader(t, dir, command)
			require.NoError(t, err, "muxt %s", command)
			assert.True(t, strings.HasPrefix(stdout, testModule+"\n"), "key line ahead of the listing in\n%s", stdout)

			stdout, _, _, err = analyzeWithLoader(t, dir, command, "--format", "json")
			require.NoError(t, err, "muxt %s --format json", command)
			var listings []struct{ Key string }
			require.NoError(t, json.Unmarshal([]byte(stdout), &listings), "output:\n%s", stdout)
			require.Len(t, listings, 1, "output:\n%s", stdout)
			assert.Equal(t, testModule, listings[0].Key, "key")
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

// twoSetModule is a module whose root package holds a public and an admin
// route set, each with its own templates variable, output file and routes
// function, so a run over it names two sets of one key.
func twoSetModule(t *testing.T) string {
	t.Helper()
	return twoSetModuleIn(t, twoSetPlace{})
}

// twoSetPlace is where twoSetModuleIn puts the two sets: in the package at
// Sub below the module root, which holds the muxt.json, and the root when Sub
// is empty.
type twoSetPlace struct{ Sub string }

// Key is the packages key of the package holding the sets.
func (p twoSetPlace) Key() string {
	if p.Sub == "" {
		return testModule
	}
	return testModule + "/" + p.Sub
}

// twoSetModuleIn is twoSetModule with the sets in the package at place. It
// returns the module root.
func twoSetModuleIn(t *testing.T, place twoSetPlace) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module "+testModule+"\n\ngo 1.24\n")
	dir := filepath.Join(root, filepath.FromSlash(place.Sub))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	writeTestFile(t, dir, "package.go", `package root

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var source embed.FS

var publicTmpl = template.Must(template.ParseFS(source, "public.gohtml"))

var adminTmpl = template.Must(template.ParseFS(source, "admin.gohtml"))
`)
	writeTestFile(t, dir, "public.gohtml", `{{define "GET /public"}}<p>public</p>{{end}}`)
	writeTestFile(t, dir, "admin.gohtml", `{{define "GET /admin"}}<p>admin</p>{{end}}`)
	writeMuxtJSON(t, root, `{"packages": {"`+place.Key()+`": [
		{"use_templates_variable": ["publicTmpl"], "output_file": "routes_public.go", "output_routes_func": "PublicRoutes"},
		{"use_templates_variable": ["adminTmpl"], "output_file": "routes_admin.go", "output_routes_func": "AdminRoutes"}
	]}}`)
	return root
}

// twoSetRuns are the directories a run over two sets of one key can start
// in: the muxt.json's own, and the directory of a package below it, which
// acts on that package's sets alone and must name them all the same.
func twoSetRuns() []struct {
	name  string
	place twoSetPlace
	run   string
} {
	return []struct {
		name  string
		place twoSetPlace
		run   string
	}{
		{name: "in the muxt.json directory", place: twoSetPlace{}, run: "."},
		{name: "in the package directory", place: twoSetPlace{Sub: "web"}, run: "web"},
	}
}

// TestCheckChecksEachSetOfAKeyAgainstItsOwnTemplates states that muxt check
// over a key with two sets checks each set's templates and no others: a mistake
// in the first set's template fails the run at that set, naming it, and one in
// the second set's template fails at the second after the first's summary.
func TestCheckChecksEachSetOfAKeyAgainstItsOwnTemplates(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	for _, tt := range []struct {
		name       string
		file       string
		wantStdout string
		failed     string
	}{
		{name: "first set", file: "public.gohtml", wantStdout: "", failed: testModule + " (routes_public.go)"},
		{name: "second set", file: "admin.gohtml", wantStdout: testModule + " (routes_public.go)\nok: 1 template\n", failed: adminSet},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := twoSetModule(t)
			_, _, err := generateWithLoader(t, root)
			require.NoError(t, err, "muxt generate")
			writeTestFile(t, root, tt.file, `{{define "GET /`+strings.TrimSuffix(tt.file, ".gohtml")+`"}}<p>{{.Missing}}</p>{{end}}`)

			stdout, stderr, _, err := analyzeWithLoader(t, root, "check")

			require.Error(t, err, "muxt check")
			assert.Equal(t, tt.wantStdout, stdout, "stdout")
			assert.Contains(t, stderr, "Error: "+tt.failed+": 1 error\n", "stderr")
		})
	}
}

// setSections splits the text a listing over twoSetModule wrote at the lines
// that name a set, and returns each set's lines by that name.
func setSections(stdout string) map[string]string {
	sections := make(map[string]string)
	var name string
	for _, line := range strings.SplitAfter(stdout, "\n") {
		switch strings.TrimSuffix(line, "\n") {
		case testModule + " (routes_public.go)", adminSet:
			name = strings.TrimSuffix(line, "\n")
			continue
		}
		sections[name] += line
	}
	return sections
}

// TestCallsListsEachSetOfAKeyItsOwnCalls states that muxt list-template-calls
// over a key with two sets lists, under each set's name, the calls of that
// set's templates and not the other's.
func TestCallsListsEachSetOfAKeyItsOwnCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	root := twoSetModule(t)
	writeTestFile(t, root, "public.gohtml", `{{define "GET /public"}}{{template "fragment"}}{{end}}{{define "fragment"}}<p>public</p>{{end}}`)
	_, _, err := generateWithLoader(t, root)
	require.NoError(t, err, "muxt generate")

	stdout, _, _, err := analyzeWithLoader(t, root, "list-template-calls")

	require.NoError(t, err, "muxt list-template-calls")
	sections := setSections(stdout)
	assert.Contains(t, sections[testModule+" (routes_public.go)"], `template "GET /public" calls:`, "public set in\n%s", stdout)
	assert.Contains(t, sections[testModule+" (routes_public.go)"], `template "fragment"`, "public set in\n%s", stdout)
	assert.NotContains(t, sections[adminSet], `fragment`, "admin set in\n%s", stdout)
	assert.NotContains(t, sections[adminSet], `GET /public`, "admin set in\n%s", stdout)
}

// TestCheckNamesEachSetOfAKeyByItsFile states that muxt check over a key with
// two sets heads each summary with the key and the set's output file, so the
// two summaries can be told apart.
func TestCheckNamesEachSetOfAKeyByItsFile(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	for _, tt := range twoSetRuns() {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(twoSetModuleIn(t, tt.place), tt.run)
			_, _, err := generateWithLoader(t, dir)
			require.NoError(t, err, "muxt generate")

			stdout, _, _, err := analyzeWithLoader(t, dir, "check")

			require.NoError(t, err, "muxt check")
			want := `\A` + regexp.QuoteMeta(tt.place.Key()+" (routes_public.go)") + `\nok: \d+ templates?\n` +
				regexp.QuoteMeta(tt.place.Key()+" (routes_admin.go)") + `\nok: \d+ templates?\n\z`
			assert.Regexp(t, want, stdout, "stdout")
		})
	}
}
