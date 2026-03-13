package generate

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/maxbrunsfeld/counterfeiter/v6/generator"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/imports"
)

// FakeServerConfig holds the configuration for generating a fake server.
type FakeServerConfig struct {
	PackagePath       string // import path of the muxt-generated package
	PackageDir        string // absolute directory of the package
	RoutesFunction    string // e.g. "TemplateRoutes"
	ReceiverInterface string // e.g. "RoutesReceiver"
	Logger            bool   // whether RoutesFunction takes *slog.Logger
	PathPrefix        bool   // whether RoutesFunction takes pathPrefix string
	FakeImportPath    string // import path of the generated fake package
}

// FakeServerFiles holds the generated files for the fake server.
type FakeServerFiles struct {
	Main []byte // main.go — small, readable entry point
	Fake []byte // internal/fake/receiver.go — counterfeiter-generated fake struct
}

// GenerateFakeServer generates two files: a main.go with the httptest server
// entry point, and a receiver.go with the counterfeiter-generated fake struct
// in a separate package.
//
// The target package must not be "main" — the fake server imports it as a library.
//
// The fake implementation interface is unstable and should not be relied upon.
func GenerateFakeServer(config FakeServerConfig, pl []*packages.Package) (*FakeServerFiles, error) {
	// Find the target package and validate it's not main.
	var targetPkg *packages.Package
	for _, pkg := range pl {
		if pkg.PkgPath == config.PackagePath {
			targetPkg = pkg
			break
		}
	}
	if targetPkg == nil {
		return nil, fmt.Errorf("package %q not found in loaded packages", config.PackagePath)
	}
	if targetPkg.Name == "main" {
		return nil, fmt.Errorf("cannot generate fake server for package %q: package is main (the target package must be a library)", config.PackagePath)
	}

	cache := &preloadedCache{
		pkgPath:  config.PackagePath,
		packages: pl,
	}

	fake, err := generator.NewFake(
		generator.InterfaceOrFunction,
		config.ReceiverInterface,
		config.PackagePath,
		config.ReceiverInterface,
		"fake",
		"",
		config.PackageDir,
		cache,
	)
	if err != nil {
		return nil, fmt.Errorf("counterfeiter: %w", err)
	}

	fakeSource, err := fake.Generate(true)
	if err != nil {
		return nil, fmt.Errorf("counterfeiter generate: %w", err)
	}

	// Generate main.go from template.
	pkgAlias := targetPkg.Name
	if mainTemplateReservedNames[pkgAlias] {
		pkgAlias += "pkg"
	}

	type mainTemplateData struct {
		FakeServerConfig
		PackageName string
	}
	var mainBuf bytes.Buffer
	if err := mainFuncTemplate.Execute(&mainBuf, mainTemplateData{
		FakeServerConfig: config,
		PackageName:      pkgAlias,
	}); err != nil {
		return nil, fmt.Errorf("executing main template: %w", err)
	}
	mainBytes, err := imports.Process("main.go", mainBuf.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("goimports main.go: %w", err)
	}

	return &FakeServerFiles{
		Main: mainBytes,
		Fake: fakeSource,
	}, nil
}

// mainTemplateReservedNames are identifiers used in mainFuncTemplate that would
// collide if the target package has the same name.
var mainTemplateReservedNames = map[string]bool{
	"context": true, "fake": true, "fmt": true, "http": true,
	"httptest": true, "main": true, "os": true, "signal": true,
	"slog": true, "syscall": true,
}

var mainFuncTemplate = template.Must(template.New("main").Parse(`package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"syscall"

	"{{.FakeImportPath}}"
	{{.PackageName}} "{{.PackagePath}}"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	receiver := new(fake.{{.ReceiverInterface}})

	mux := http.NewServeMux()
	{{.PackageName}}.{{.RoutesFunction}}(mux, receiver{{if .Logger}}, slog.Default(){{end}}{{if .PathPrefix}}, ""{{end}})

	server := httptest.NewServer(mux)
	defer server.Close()
	fmt.Println("Explore at:", server.URL)

	<-ctx.Done()
}
`))

// preloadedCache implements generator.Cacher to pass already-loaded packages
// to counterfeiter, avoiding a redundant packages.Load call.
type preloadedCache struct {
	pkgPath  string
	packages []*packages.Package
}

func (c *preloadedCache) Load(path string) ([]*packages.Package, bool) {
	if path == c.pkgPath {
		return c.packages, true
	}
	return nil, false
}

func (c *preloadedCache) Store(string, []*packages.Package) {}
