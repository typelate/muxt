package load_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/load/loadtest"
)

// TestCheckFileRead states which file names the templates variables of a
// package read, judged from their //go:embed and ParseFS patterns.
func TestCheckFileRead(t *testing.T) {
	for _, tt := range []struct {
		name      string
		source    string
		variables []string
		file      string
		want      *load.UnreadFileError
	}{
		{
			name: "both patterns match",
			source: `//go:embed *.gohtml
var fs embed.FS

var templates = template.Must(template.ParseFS(fs, "*.gohtml"))
`,
			file: "extra.gohtml",
		},
		{
			name: "the embed pattern misses",
			source: `//go:embed page.gohtml
var fs embed.FS

var templates = template.Must(template.ParseFS(fs, "*.gohtml"))
`,
			file: "extra.gohtml",
			want: &load.UnreadFileError{Variable: "templates", EmbedPatterns: []string{"page.gohtml"}, ParsePatterns: []string{"*.gohtml"}},
		},
		{
			name: "the ParseFS pattern misses",
			source: `//go:embed *.gohtml
var fs embed.FS

var templates = template.Must(template.ParseFS(fs, "page.gohtml"))
`,
			file: "extra.gohtml",
			want: &load.UnreadFileError{Variable: "templates", EmbedPatterns: []string{"*.gohtml"}, ParsePatterns: []string{"page.gohtml"}},
		},
		{
			name: "a pattern that is not a literal could match",
			source: `//go:embed *.gohtml
var fs embed.FS

var pattern = "page.gohtml"

var templates = template.Must(template.ParseFS(fs, pattern))
`,
			file: "extra.gohtml",
		},
		{
			name: "the second of several ParseFS calls reads it",
			source: `//go:embed page.gohtml
var pages embed.FS

//go:embed *.gohtml
var all embed.FS

var templates = template.Must(template.Must(template.ParseFS(pages, "*.gohtml")).ParseFS(all, "extra.gohtml"))
`,
			file: "extra.gohtml",
		},
		{
			name: "none of several ParseFS calls reads it",
			source: `//go:embed page.gohtml
var pages embed.FS

//go:embed *.gohtml
var all embed.FS

var templates = template.Must(template.Must(template.ParseFS(pages, "*.gohtml")).ParseFS(all, "page.gohtml"))
`,
			file: "extra.gohtml",
			want: &load.UnreadFileError{Variable: "templates", EmbedPatterns: []string{"*.gohtml", "page.gohtml"}, ParsePatterns: []string{"page.gohtml", "*.gohtml"}},
		},
		{
			name: "the second of several variables reads it",
			source: `//go:embed *.gohtml
var fs embed.FS

var pages = template.Must(template.ParseFS(fs, "page.gohtml"))

var extras = template.Must(template.ParseFS(fs, "extra.gohtml"))
`,
			variables: []string{"pages", "extras"},
			file:      "extra.gohtml",
		},
		{
			name: "none of several variables reads it",
			source: `//go:embed *.gohtml
var fs embed.FS

var pages = template.Must(template.ParseFS(fs, "page.gohtml"))

var others = template.Must(template.ParseFS(fs, "other.gohtml"))
`,
			variables: []string{"pages", "others"},
			file:      "extra.gohtml",
			want:      &load.UnreadFileError{Variable: "pages", EmbedPatterns: []string{"*.gohtml"}, ParsePatterns: []string{"page.gohtml"}},
		},
		{
			name: "a quoted embed pattern may hold a space",
			source: `//go:embed "my page.gohtml"
var fs embed.FS

var templates = template.Must(template.ParseFS(fs, "my page.gohtml"))
`,
			file: "my page.gohtml",
		},
		{
			name: "an all: prefix is ignored for a file in the directory",
			source: `//go:embed all:*.gohtml
var fs embed.FS

var templates = template.Must(template.ParseFS(fs, "*.gohtml"))
`,
			file: "extra.gohtml",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			pl := loadtest.Package(t, dir, "example.com/server", map[string]string{
				"server.go":   "package server\n\nimport (\n\t\"embed\"\n\t\"html/template\"\n)\n\n" + tt.source,
				"page.gohtml": `{{define "page"}}{{end}}`,
			})
			variables := tt.variables
			if variables == nil {
				variables = []string{"templates"}
			}

			err := load.CheckFileRead(dir, pl, variables, tt.file)

			if tt.want == nil {
				assert.NoError(t, err)
				return
			}
			tt.want.File = filepath.Join(dir, tt.file)
			var unread *load.UnreadFileError
			require.True(t, errors.As(err, &unread), "CheckFileRead(%q) = %v, want an *UnreadFileError", tt.file, err)
			assert.Equal(t, tt.want, unread)
		})
	}
}

// TestCheckFileReadWithAnotherPackagesFS states that a ParseFS call reading
// another package's embed.FS reads a file in the working directory only when
// that package's directory holds it. The package is parsed by hand, because
// loadtest type checks against the standard library alone.
func TestCheckFileReadWithAnotherPackagesFS(t *testing.T) {
	const source = `package contacts

import (
	"embed"
	"html/template"

	"example.com/app/fragments"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.Must(template.New("").
	ParseFS(fragments.TemplateSource, "*.gohtml")).
	ParseFS(templateSource, "contacts.gohtml"))
`
	root := t.TempDir()
	for _, tt := range []struct {
		name         string
		fragmentsDir string
		want         *load.UnreadFileError
	}{
		{
			name:         "a sibling package's FS cannot read it",
			fragmentsDir: filepath.Join(root, "fragments"),
			want: &load.UnreadFileError{
				File:          filepath.Join(root, "contacts", "extra.gohtml"),
				Variable:      "templates",
				EmbedPatterns: []string{"*.gohtml"},
				ParsePatterns: []string{"contacts.gohtml"},
			},
		},
		{
			name:         "an ancestor package's FS could read it",
			fragmentsDir: root,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wd := filepath.Join(root, "contacts")
			fset := token.NewFileSet()
			goFile := filepath.Join(wd, "contacts.go")
			file, err := parser.ParseFile(fset, goFile, source, parser.ParseComments|parser.SkipObjectResolution)
			require.NoError(t, err)
			pl := []*packages.Package{{
				Name:    "contacts",
				PkgPath: "example.com/app/contacts",
				GoFiles: []string{goFile},
				Fset:    fset,
				Syntax:  []*ast.File{file},
				Imports: map[string]*packages.Package{
					"example.com/app/fragments": {
						Name:    "fragments",
						PkgPath: "example.com/app/fragments",
						GoFiles: []string{filepath.Join(tt.fragmentsDir, "fragments.go")},
					},
				},
			}}

			err = load.CheckFileRead(wd, pl, []string{"templates"}, "extra.gohtml")

			if tt.want == nil {
				assert.NoError(t, err)
				return
			}
			var unread *load.UnreadFileError
			require.True(t, errors.As(err, &unread), "CheckFileRead(%q) = %v, want an *UnreadFileError", "extra.gohtml", err)
			assert.Equal(t, tt.want, unread)
		})
	}
}
