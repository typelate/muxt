package load

import (
	"path/filepath"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxt"
)

// This file hydrates a command's configuration: it reads, from the packages
// loaded for the working directory, what the configuration names, and
// returns it as the input the command's implementation runs on.

// GenerateSource hydrates muxt generate's configuration: the package the
// routes file is written into, the receiver type it names, and its
// templates variables.
//
// The routes file belongs to the package in its own directory, which is
// the working directory unless the output file names another.
func GenerateSource(wd string, pl []*packages.Package, config generate.RoutesFileConfiguration) (muxt.Source, error) {
	return Source(filepath.Dir(filepath.Join(wd, config.OutputFileName)), pl, SourceConfiguration{
		ReceiverType:       config.ReceiverType,
		ReceiverPackage:    config.ReceiverPackage,
		TemplatesVariables: config.TemplatesVariables,
	})
}

// RoutesSource hydrates the route listing's configuration.
func RoutesSource(wd string, pl []*packages.Package, config analysis.DefinitionsConfiguration) (muxt.Source, error) {
	return Source(wd, pl, SourceConfiguration{
		ReceiverType:       config.ReceiverType,
		ReceiverPackage:    config.ReceiverPackage,
		TemplatesVariables: config.TemplatesVariables,
	})
}
