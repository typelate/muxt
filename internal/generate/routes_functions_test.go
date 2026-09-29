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
	if err != nil {
		t.Fatal(err)
	}
	return src, defs
}

func TestGroupTemplates(t *testing.T) {
	_, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml":   `{{define "GET /a/{id} A(id)"}}{{end}}`,
		"b.gohtml":   `{{define "GET /b B()"}}{{end}}{{define "GET /b2 B()"}}{{end}}`,
		"c d.gohtml": `{{define "GET /c B()"}}{{end}}`,
	})
	groups, err := groupTemplates(testConfig(), defs)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := slices.Sorted(maps.Keys(groups.byFile)), []string{"a.gohtml", "b.gohtml"}; !slices.Equal(got, want) {
		t.Errorf("groups.byFile keys = %q, want %q", got, want)
	}
	if got := len(groups.byFile["b.gohtml"]); got != 2 {
		t.Errorf("len(groups.byFile[b.gohtml]) = %d, want 2", got)
	}
	if len(groups.noFile) != 1 || groups.noFile[0].RawPattern() != "GET /c" {
		t.Errorf("groups.noFile = %v, want the route of the file with a space in its name", groups.noFile)
	}
	if len(groups.all) != len(defs) || len(groups.all) != 4 {
		t.Errorf("len(groups.all) = %d, want 4", len(groups.all))
	}
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
				if !strings.Contains(buf.String(), want) {
					t.Errorf("log = %q, want it to contain %q", buf.String(), want)
				}
			}
			for _, unwanted := range tt.excludes {
				if strings.Contains(buf.String(), unwanted) {
					t.Errorf("log = %q, want it not to contain %q", buf.String(), unwanted)
				}
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
	if err := collectReceiverMethods(defs, newFile(pkg), receiverInterface); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, field := range receiverInterface.Methods.List {
		names = append(names, field.Names[0].Name)
	}
	slices.Sort(names)
	if want := []string{"A", "B", "Nested"}; !slices.Equal(names, want) {
		t.Errorf("collectReceiverMethods interface methods = %q, want %q (each method once, nested calls included)", names, want)
	}
	if got := astgen.Format(receiverInterface); !strings.Contains(got, "Nested(string) string") {
		t.Errorf("receiver interface = %s, want it to declare Nested(string) string", got)
	}
}

func TestGeneratePerFileRouteFunction(t *testing.T) {
	pkg, defs := routesTestDefinitions(t, map[string]string{
		"a.gohtml": `{{define "GET /a/{id} A(id)"}}{{end}}{{define "GET /plain"}}{{end}}`,
	})

	t.Run("source file is required", func(t *testing.T) {
		_, err := generatePerFileRouteFunction("", defs, newFile(pkg), "aRoutes", "aReceiver", log.New(&bytes.Buffer{}, "", 0), testConfig(), &ast.InterfaceType{Methods: new(ast.FieldList)})
		if err == nil || err.Error() != "sourceFile cannot be empty" {
			t.Errorf("error = %v, want sourceFile cannot be empty", err)
		}
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
			if err != nil {
				t.Fatal(err)
			}
			got := astgen.Format(decl)
			if first, _, _ := strings.Cut(got, "\n"); first != tt.signature {
				t.Errorf("signature = %q, want %q", first, tt.signature)
			}
			if !strings.Contains(got, "bytesBufferPool") {
				t.Errorf("function has no buffer pool declaration:\n%s", got)
			}
			for _, want := range []string{"generating handler for pattern GET /a/{id} in a.gohtml", "generating handler for pattern GET /plain in a.gohtml"} {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("log = %q, want it to contain %q", buf.String(), want)
				}
			}
			if len(iface.Methods.List) != 1 || iface.Methods.List[0].Names[0].Name != "A" {
				t.Errorf("receiver interface = %s, want it to declare A", astgen.Format(iface))
			}
		})
	}

	t.Run("no routes has no buffer pool", func(t *testing.T) {
		decl, err := generatePerFileRouteFunction("a.gohtml", nil, newFile(pkg), "aRoutes", "aReceiver", nil, testConfig(), &ast.InterfaceType{Methods: new(ast.FieldList)})
		if err != nil {
			t.Fatal(err)
		}
		if got := astgen.Format(decl); strings.Contains(got, "bytesBufferPool") {
			t.Errorf("function with no routes declares a buffer pool:\n%s", got)
		}
	})
}
