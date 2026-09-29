package generate

import (
	"bytes"
	"go/ast"
	"go/types"
	"html/template"
	"log"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

const routesTestSource = `package server

type T struct{}

func (T) A(id int) int { return 0 }

func (T) B() string { return "" }

func (T) Nested(string) string { return "" }

func (T) Two(a, b string) string { return "" }

func (T) Run(execute func() error) string { return "" }

type ResponseWriter interface{ Write([]byte) (int, error) }

func (T) Raw(ResponseWriter) string { return "" }
`

// routesTestDefinitions resolves the routes of files, one template file per
// entry, over routesTestSource. A file name with a space is a template that
// no single file declares.
func routesTestDefinitions(t *testing.T, files map[string]string) (source.Package, []muxt.Definition) {
	t.Helper()
	pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": routesTestSource})
	set := template.New("templates")
	for _, name := range slices.Sorted(maps.Keys(files)) {
		template.Must(set.New(name).Parse(files[name]))
	}
	src := source.Package{
		Fset:      fake.FileSet,
		Types:     pkg,
		Variables: []source.Variable{{Name: "templates", Set: set}},
	}
	receiver := fake.Lookup(t, pkg, "T").(*types.Named)
	defs, err := muxt.ResolveDefinitions(src, receiver, fake.StandInChecker(t, pkg).Fake())
	require.NoError(t, err)
	return src, defs
}

func TestGroupTemplates(t *testing.T) {
	_, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml":   `{{define "GET /a/{id} A(id)"}}{{end}}`,
		"b.gohtml":   `{{define "GET /b B()"}}{{end}}{{define "GET /b2 B()"}}{{end}}`,
		"c d.gohtml": `{{define "GET /c B()"}}{{end}}`,
	})
	groups, err := groupTemplates(testConfig(), defs)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.gohtml", "b.gohtml"}, slices.Sorted(maps.Keys(groups.byFile)), "groups.byFile keys")
	assert.Len(t, groups.byFile["b.gohtml"], 2, "groups.byFile[b.gohtml]")
	if assert.Len(t, groups.noFile, 1, "groups.noFile, want the route of the file with a space in its name") {
		assert.Equal(t, "GET /c", groups.noFile[0].RawPattern(), "groups.noFile[0]")
	}
	assert.Len(t, groups.all, len(defs), "groups.all, want every definition")
	assert.Len(t, groups.all, 4, "groups.all")
}

func TestLogResolutionNotes(t *testing.T) {
	_, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /raw Raw(response)"}}{{end}}{{define "GET /missing Missing()"}}{{end}}`,
	})
	const responseWarning = "warning: GET /raw uses the response argument"
	for _, tt := range []struct {
		name     string
		config   func(RoutesFileConfiguration) RoutesFileConfiguration
		contains []string
		excludes []string
	}{
		{
			name:     "no receiver type only warns about the response argument",
			config:   func(c RoutesFileConfiguration) RoutesFileConfiguration { return c },
			contains: []string{responseWarning},
			excludes: []string{"note:"},
		},
		{
			name:     "silenced warning",
			config:   func(c RoutesFileConfiguration) RoutesFileConfiguration { c.SilenceHTTPResponseWarning = true; return c },
			excludes: []string{"warning:", "note:"},
		},
		{
			name:     "receiver type lists synthesized methods once explained",
			config:   func(c RoutesFileConfiguration) RoutesFileConfiguration { c.ReceiverType = "T"; return c },
			contains: []string{responseWarning, "note: T does not define ", "Missing", "note: the inferred signatures return any"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logResolutionNotes(defs, tt.config(testConfig()), log.New(&buf, "", 0))
			for _, want := range tt.contains {
				assert.Contains(t, buf.String(), want, "log")
			}
			for _, unwanted := range tt.excludes {
				assert.NotContains(t, buf.String(), unwanted, "log")
			}
		})
	}

	t.Run("nil logger", func(t *testing.T) {
		config := testConfig()
		config.ReceiverType = "T"
		logResolutionNotes(defs, config, nil)
	})
}

func TestCollectReceiverMethods(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /a/{id} A(id)"}}{{end}}{{define "GET /b B()"}}{{end}}{{define "GET /n Nested(B())"}}{{end}}{{define "GET /plain"}}{{end}}`,
	})
	receiverInterface := &ast.InterfaceType{Methods: new(ast.FieldList)}
	require.NoError(t, collectReceiverMethods(defs, newFile(pkg), receiverInterface))
	var names []string
	for _, field := range receiverInterface.Methods.List {
		names = append(names, field.Names[0].Name)
	}
	slices.Sort(names)
	assert.Equal(t, []string{"A", "B", "Nested"}, names, "collectReceiverMethods interface methods, each method once, nested calls included")
	assert.Contains(t, astgen.Format(receiverInterface), "Nested(string) string", "receiver interface")
}

func TestGeneratePerFileRouteFunction(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /a/{id} A(id)"}}{{end}}{{define "GET /plain"}}{{end}}`,
	})

	t.Run("source file is required", func(t *testing.T) {
		_, err := generatePerFileRouteFunction("", defs, newFile(pkg), "aRoutes", "aReceiver", log.New(&bytes.Buffer{}, "", 0), testConfig(), &ast.InterfaceType{Methods: new(ast.FieldList)})
		assert.EqualError(t, err, "sourceFile cannot be empty")
	})

	for _, tt := range []struct {
		name      string
		configure func(*RoutesFileConfiguration)
		signature string
	}{
		{
			name:      "minimal",
			configure: func(*RoutesFileConfiguration) {},
			signature: "func aRoutes(mux *http.ServeMux, receiver aReceiver, pathsPrefix string) {",
		},
		{
			name: "logger and middleware",
			configure: func(c *RoutesFileConfiguration) {
				c.Logger = true
				c.Middleware = true
			},
			signature: "func aRoutes(mux *http.ServeMux, receiver aReceiver, logger *slog.Logger, pathsPrefix string, middleware func(next http.Handler) http.Handler) {",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig()
			config.Verbose = true
			tt.configure(&config)
			var buf bytes.Buffer
			iface := &ast.InterfaceType{Methods: new(ast.FieldList)}
			decl, err := generatePerFileRouteFunction("a.gohtml", defs, newFile(pkg), "aRoutes", "aReceiver", log.New(&buf, "", 0), config, iface)
			require.NoError(t, err)
			got := astgen.Format(decl)
			first, _, _ := strings.Cut(got, "\n")
			assert.Equal(t, tt.signature, first, "signature")
			assert.Contains(t, got, "bytesBufferPool", "function should declare a buffer pool")
			for _, want := range []string{"generating handler for pattern GET /a/{id} in a.gohtml", "generating handler for pattern GET /plain in a.gohtml"} {
				assert.Contains(t, buf.String(), want, "log")
			}
			if assert.Len(t, iface.Methods.List, 1, "receiver interface = %s, want it to declare A", astgen.Format(iface)) {
				assert.Equal(t, "A", iface.Methods.List[0].Names[0].Name, "receiver interface method")
			}
		})
	}

	t.Run("no routes has no buffer pool", func(t *testing.T) {
		decl, err := generatePerFileRouteFunction("a.gohtml", nil, newFile(pkg), "aRoutes", "aReceiver", nil, testConfig(), &ast.InterfaceType{Methods: new(ast.FieldList)})
		require.NoError(t, err)
		assert.NotContains(t, astgen.Format(decl), "bytesBufferPool", "function with no routes should not declare a buffer pool")
	})
}
