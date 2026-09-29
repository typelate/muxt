package fakeserver

import (
	"bytes"
	"fmt"
	"go/format"
	"text/template"

	"github.com/maxbrunsfeld/counterfeiter/v6/generator"
	"golang.org/x/tools/go/packages"
)

// Config holds the configuration for generating a fake server.
type Config struct {
	PackagePath       string // import path of the muxt-generated package
	PackageDir        string // absolute directory of the package
	RoutesFunction    string // e.g. "TemplateRoutes"
	ReceiverInterface string // e.g. "RoutesReceiver"
	Logger            bool   // whether RoutesFunction takes *slog.Logger
	PathPrefix        bool   // whether RoutesFunction takes pathPrefix string
	Middleware        bool   // whether RoutesFunction takes middleware func(next http.Handler) http.Handler
	FakeImportPath    string // import path of the generated fake package
}

// Files holds the generated files for the fake server.
type Files struct {
	Main []byte // main.go — small, readable entry point
	Fake []byte // internal/fake/receiver.go — counterfeiter-generated fake struct
}

// Generate generates two files: a main.go with the httptest server
// entry point, and a receiver.go with the counterfeiter-generated fake struct
// in a separate package.
//
// The target package must not be "main" — the fake server imports it as a library.
//
// The fake implementation interface is unstable and should not be relied upon.
func Generate(config Config, pl []*packages.Package) (*Files, error) {
	targetPkg, err := libraryPackage(pl, config.PackagePath)
	if err != nil {
		return nil, err
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

	mainSource, err := renderMain(config, targetPkg.Name)
	if err != nil {
		return nil, err
	}

	return &Files{
		Main: mainSource,
		Fake: fakeSource,
	}, nil
}

func libraryPackage(pl []*packages.Package, path string) (*packages.Package, error) {
	for _, pkg := range pl {
		if pkg.PkgPath != path {
			continue
		}
		if pkg.Name == "main" {
			return nil, fmt.Errorf("cannot generate fake server for package %q: package is main (the target package must be a library)", path)
		}
		return pkg, nil
	}
	return nil, fmt.Errorf("package %q not found in loaded packages", path)
}

func renderMain(config Config, packageName string) ([]byte, error) {
	if mainTemplateReservedNames[packageName] {
		packageName += "pkg"
	}
	var buf bytes.Buffer
	err := mainFuncTemplate.Execute(&buf, struct {
		Config
		PackageName string
	}{Config: config, PackageName: packageName})
	if err != nil {
		return nil, fmt.Errorf("executing main template: %w", err)
	}
	source, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting main.go: %w", err)
	}
	return source, nil
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
	{{.PackageName}}.{{.RoutesFunction}}(mux, receiver{{if .Logger}}, slog.Default(){{end}}{{if .PathPrefix}}, ""{{end}}{{if .Middleware}}, nil{{end}})

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
