package analysis

import (
	"go/token"
	"go/types"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
)

// TestCheckUnusedTemplatesOfSharedFiles states which unused templates check
// reports when another package's files were parsed into the set: a template
// from a shared file is left to the package that owns the file, one from the
// package's own file is reported, and a route template from a shared file is
// reported as unwired, since the package generates the handlers of the routes
// it parses. Definition.Package is what tells them apart.
func TestCheckUnusedTemplatesOfSharedFiles(t *testing.T) {
	const fragments = "example.com/m/fragments"
	for _, tt := range []struct {
		name          string
		templates     string
		sharedNames   []string
		wantErr       string
		wantLogged    []string
		wantNotLogged []string
	}{
		{
			name:          "an unused shared template is not reported",
			templates:     `{{define "footer"}}<footer></footer>{{end}}`,
			sharedNames:   []string{"footer"},
			wantNotLogged: []string{"Unused templates", `"footer"`},
		},
		{
			name:       "an unused template of the package's own file is reported",
			templates:  `{{define "orphan"}}<i></i>{{end}}`,
			wantErr:    "1 error",
			wantLogged: []string{"Unused templates:", `"orphan"`},
		},
		{
			name:          "only the own one when both are unused",
			templates:     `{{define "footer"}}<footer></footer>{{end}}{{define "orphan"}}<i></i>{{end}}`,
			sharedNames:   []string{"footer"},
			wantErr:       "1 error",
			wantLogged:    []string{"Unused templates:", `"orphan"`},
			wantNotLogged: []string{`"footer"`},
		},
		{
			name:        "a route template of a shared file is reported as unwired",
			templates:   `{{define "GET /shared SharedPage()"}}<p></p>{{end}}`,
			sharedNames: []string{"GET /shared SharedPage()"},
			wantErr:     "1 error",
			wantLogged:  []string{"Route templates with no generated handler", `"GET /shared SharedPage()"`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			set := parseTemplates(t, tt.templates)
			definitions := make(map[string]source.Definition)
			for _, t := range set.Templates() {
				if t.Tree != nil && t.Name() != set.Name() {
					definitions[t.Name()] = source.Definition{Name: t.Name(), Tree: t.Tree}
				}
			}
			for _, name := range tt.sharedNames {
				d, ok := definitions[name]
				require.True(t, ok, "the premise of this test is wrong: no template %q", name)
				d.Package = fragments
				definitions[name] = d
			}
			pkg := source.Package{
				Fset:      token.NewFileSet(),
				Types:     types.NewPackage("example.com/m/workspace", "workspace"),
				Variables: []source.Variable{{Name: "templates", Set: set, Definitions: definitions}},
			}
			var logs strings.Builder

			_, err := Check(CheckConfiguration{}, log.New(&logs, "", 0), pkg)

			if tt.wantErr == "" {
				require.NoError(t, err, "Check() logged %q", logs.String())
			} else {
				require.EqualError(t, err, tt.wantErr, "Check() logged %q", logs.String())
			}
			for _, want := range tt.wantLogged {
				assert.Contains(t, logs.String(), want, "Check() log")
			}
			for _, unwanted := range tt.wantNotLogged {
				assert.NotContains(t, logs.String(), unwanted, "Check() log")
			}
		})
	}
}
