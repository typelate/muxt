package load

import (
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/source"
)

// This file hydrates a command's configuration: it reads, from the packages
// loaded for the working directory, what the configuration names, and
// returns it as the input the command's implementation runs on.

// GenerateSource hydrates muxt generate's configuration: the package the
// routes file is written into, with its templates variables, and the
// receiver type the configuration names, nil when it names none.
//
// The routes file belongs to the package in its own directory, which is
// the working directory unless the output file names another.
func GenerateSource(wd string, pl []*packages.Package, config generate.RoutesFileConfiguration) (source.Package, *types.Named, error) {
	return packageWithReceiver(filepath.Dir(filepath.Join(wd, config.OutputFileName)), pl, config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
}

// RoutesSource hydrates the route listing's configuration.
func RoutesSource(wd string, pl []*packages.Package, config analysis.DefinitionsConfiguration) (source.Package, *types.Named, error) {
	return packageWithReceiver(wd, pl, config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
}

// packageWithReceiver reads the package at dir and the receiver type,
// reporting a missing package before a missing receiver, and either before
// a templates variable that does not evaluate.
func packageWithReceiver(dir string, pl []*packages.Package, receiverPackage, receiverType string, variables []string) (source.Package, *types.Named, error) {
	if _, ok := PackageAtFilepath(pl, dir); !ok {
		return source.Package{}, nil, NoPackageError(dir, pl)
	}
	var receiver *types.Named
	if receiverType != "" {
		var err error
		if receiver, err = Receiver(dir, pl, receiverPackage, receiverType); err != nil {
			return source.Package{}, nil, err
		}
	}
	pkg, err := Package(dir, pl, variables)
	if err != nil {
		return source.Package{}, nil, err
	}
	return pkg, receiver, nil
}
