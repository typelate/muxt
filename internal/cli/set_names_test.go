package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/load"
)

// adminSet is how a run names the second set of twoSetModule: the key and the
// file, since the key alone also names the public set.
const adminSet = testModule + " (routes_admin.go)"

// TestRunsNameTheSetOfAKeyWithSeveralSets states that every place a run says
// whose output it is, a failure's prefix, a diagnostics heading, a note's
// prefix, prints "KEY (FILE)" for a key with two sets and never the bare key,
// which would name both.
func TestRunsNameTheSetOfAKeyWithSeveralSets(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	for _, tt := range []struct {
		name   string
		change func(t *testing.T, root string)
		run    func(t *testing.T, root string) (stdout, stderr string, err error)
		check  func(t *testing.T, stdout, stderr string, err error)
	}{
		{
			name: "generate failure prefix",
			change: func(t *testing.T, root string) {
				writeTestFile(t, root, "admin.gohtml", `{{define "GET /admin Missing(ctx"}}<p>admin</p>{{end}}`)
			},
			run: generateRun,
			check: func(t *testing.T, _, _ string, err error) {
				require.Error(t, err, "generate")
				assert.True(t, strings.HasPrefix(err.Error(), adminSet+": "), "error %q does not start with %q", err, adminSet+": ")
			},
		},
		{
			name: "generate diagnostics heading",
			change: func(t *testing.T, root string) {
				writeTestFile(t, root, "admin.gohtml", `{{define "GET /admin Missing(ctx"}}<p>admin</p>{{end}}`)
			},
			run: generateRun,
			check: func(t *testing.T, _, stderr string, err error) {
				require.Error(t, err, "generate")
				assert.True(t, strings.HasPrefix(stderr, adminSet+":\n"), "stderr does not start with %q:\n%s", adminSet+":", stderr)
			},
		},
		{
			name: "generate notes prefix",
			change: func(t *testing.T, root string) {
				writeTestFile(t, root, "receiver.go", "package root\n\ntype S struct{}\n")
				writeTestFile(t, root, "admin.gohtml", `{{define "GET /admin/{id} Missing(ctx, id)"}}<p>admin</p>{{end}}`)
				writeMuxtJSON(t, root, `{"packages": {"`+testModule+`": [
					{"use_templates_variable": ["publicTmpl"], "output_file": "routes_public.go", "output_routes_func": "PublicRoutes", "output_receiver_interface": "PublicReceiver", "output_template_data_type": "PublicData", "output_template_route_paths_type": "PublicPaths"},
					{"use_templates_variable": ["adminTmpl"], "use_receiver_type": "S", "output_file": "routes_admin.go", "output_routes_func": "AdminRoutes", "output_receiver_interface": "AdminReceiver", "output_template_data_type": "AdminData", "output_template_route_paths_type": "AdminPaths"}
				]}}`)
			},
			run: generateRun,
			check: func(t *testing.T, stdout, _ string, err error) {
				require.NoError(t, err, "generate")
				assert.Regexp(t, `(?m)^`+regexp.QuoteMeta(adminSet)+`: note: `, stdout, "stdout")
			},
		},
		{
			name: "check failure prefix and diagnostics heading",
			change: func(t *testing.T, root string) {
				_, _, err := generateWithLoader(t, root)
				require.NoError(t, err, "muxt generate")
				writeTestFile(t, root, "admin.gohtml", `{{define "GET /admin"}}<p>{{.Missing}}</p>{{end}}`)
			},
			run: func(t *testing.T, root string) (string, string, error) {
				stdout, stderr, _, err := analyzeWithLoader(t, root, "check")
				return stdout, stderr, err
			},
			check: func(t *testing.T, stdout, stderr string, err error) {
				require.Error(t, err, "check")
				assert.Equal(t, testModule+" (routes_public.go)\nok: 1 template\n", stdout, "stdout")
				assert.True(t, strings.HasPrefix(stderr, adminSet+"\n"), "stderr does not start with %q:\n%s", adminSet, stderr)
				assert.Contains(t, stderr, "Error: "+adminSet+": 1 error\n", "stderr")
			},
		},
		{
			name: "listing failure prefix",
			change: func(t *testing.T, root string) {
				writeMuxtJSON(t, root, `{"packages": {"`+testModule+`": [
					{"use_templates_variable": ["publicTmpl"], "output_file": "routes_public.go", "output_routes_func": "PublicRoutes", "output_receiver_interface": "PublicReceiver", "output_template_data_type": "PublicData", "output_template_route_paths_type": "PublicPaths"},
					{"use_templates_variable": ["missingTmpl"], "output_file": "routes_admin.go", "output_routes_func": "AdminRoutes", "output_receiver_interface": "AdminReceiver", "output_template_data_type": "AdminData", "output_template_route_paths_type": "AdminPaths"}
				]}}`)
			},
			run: func(t *testing.T, root string) (string, string, error) {
				stdout, stderr, _, err := analyzeWithLoader(t, root, "list-template-calls")
				return stdout, stderr, err
			},
			check: func(t *testing.T, _, _ string, err error) {
				require.EqualError(t, err, adminSet+": variable missingTmpl not found in package "+testModule, "list-template-calls")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := twoSetModule(t)
			tt.change(t, root)

			stdout, stderr, err := tt.run(t, root)

			tt.check(t, stdout, stderr, err)
		})
	}
}

// generateRun is muxt generate in root, the shape TestRunsNameTheSetOfAKeyWithSeveralSets takes.
func generateRun(t *testing.T, root string) (stdout, stderr string, err error) {
	t.Helper()
	stdout, stderr, _, err = generateWithLoaderOutput(t, root)
	return stdout, stderr, err
}

// TestAReloadFailureNamesTheSetOfAKeyWithSeveralSets states that when the
// reload a set needs, because it imports a package a run just wrote, fails,
// the error is prefixed with "KEY (FILE)" for a key with two sets.
func TestAReloadFailureNamesTheSetOfAKeyWithSeveralSets(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	root := entryModule(t, map[string]string{"lib": "Lib"}, nil)
	web := filepath.Join(root, "web")
	require.NoError(t, os.Mkdir(web, 0o755))
	writeTestFile(t, web, "package.go", `package web

import (
	"embed"
	"html/template"

	_ "example.com/m/lib"
)

//go:embed *.gohtml
var source embed.FS

var publicTmpl = template.Must(template.ParseFS(source, "public.gohtml"))

var adminTmpl = template.Must(template.ParseFS(source, "admin.gohtml"))
`)
	writeTestFile(t, web, "public.gohtml", `{{define "GET /public"}}<p>public</p>{{end}}`)
	writeTestFile(t, web, "admin.gohtml", `{{define "GET /admin"}}<p>admin</p>{{end}}`)
	writeMuxtJSON(t, root, `{"packages": {
		"example.com/m/lib": [{"use_receiver_type": "Lib"}],
		"example.com/m/web": [
			{"use_templates_variable": ["publicTmpl"], "output_file": "routes_public.go", "output_routes_func": "PublicRoutes", "output_receiver_interface": "PublicReceiver", "output_template_data_type": "PublicData", "output_template_route_paths_type": "PublicPaths"},
			{"use_templates_variable": ["adminTmpl"], "output_file": "routes_admin.go", "output_routes_func": "AdminRoutes", "output_receiver_interface": "AdminReceiver", "output_template_data_type": "AdminData", "output_template_route_paths_type": "AdminPaths"}
		]}}`)
	loads := 0
	failing := func(dir string, dirs, patterns []string) (*load.Loaded, error) {
		loads++
		if loads > 1 {
			return nil, errors.New("reload refused")
		}
		return load.InDirectories(dir, dirs, patterns)
	}

	err := commands(root, []string{"generate"}, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: generateRunner(failing),
	})

	require.EqualError(t, err, "example.com/m/web (routes_public.go): reload refused", "generate")
}
