package load

import (
	"cmp"
	"go/token"
	"go/types"
	"html/template"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/check"
	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/muxt"
)

// SourceConfiguration names what Source reads from the loaded packages.
type SourceConfiguration struct {
	// ReceiverType names the receiver type; when it is empty no
	// receiver is looked up.
	ReceiverType string

	// ReceiverPackage is the import path of the package declaring
	// ReceiverType. It defaults to the package in the working directory.
	ReceiverPackage string

	// TemplatesVariables are loaded in order.
	TemplatesVariables []string
}

// Source adapts the packages loaded for wd into what route resolution
// reads. It is where a go/packages result stops: past it, a run holds
// go/types values and template sets and nothing that needs the go
// command.
//
// A templates variable that fails to load does not fail Source; its
// error is carried on the variable, so a caller reports it at the point
// it reaches that variable.
func Source(wd string, pl []*packages.Package, config SourceConfiguration) (muxt.Source, error) {
	pkg, ok := PackageAtFilepath(pl, wd)
	if !ok {
		return muxt.Source{}, NoPackageError(wd, pl)
	}
	src := muxt.Source{Package: Package(pkg, pl)}
	if config.ReceiverType != "" {
		receiver, err := FindType(pl, cmp.Or(config.ReceiverPackage, pkg.PkgPath), config.ReceiverType)
		if err != nil {
			return muxt.Source{}, err
		}
		src.Receiver = receiver
	}
	for _, variable := range config.TemplatesVariables {
		src.Templates = append(src.Templates, TemplatesVariable(pkg, variable))
	}
	return src, nil
}

// Package returns pkg as route resolution reads it. Lookup finds any
// package in pl, or one a package in pl imports.
func Package(pkg *packages.Package, pl []*packages.Package) muxt.Package {
	return muxt.Package{
		Fset:  pkg.Fset,
		Types: pkg.Types,
		Lookup: func(path string) (*types.Package, bool) {
			return findPackageTypes(pl, path)
		},
	}
}

// TemplatesVariable evaluates the templates variable in pkg.
func TemplatesVariable(pkg *packages.Package, variable string) muxt.Templates {
	lt, ts, err := HTMLTemplates(variable, pkg)
	if err != nil {
		return muxt.Templates{Variable: variable, Err: err}
	}
	return templates(variable, lt, ts)
}

// AnalysisTemplates evaluates the templates variable in pkg along with
// what type checking its templates needs.
func AnalysisTemplates(pkg *packages.Package, variable string) analysis.Templates {
	lt, ts, err := HTMLTemplates(variable, pkg)
	if err != nil {
		return analysis.Templates{Templates: muxt.Templates{Variable: variable, Err: err}}
	}
	return analysis.Templates{
		Templates:   templates(variable, lt, ts),
		Trees:       lt,
		Definitions: lt,
		Functions:   lt.Functions(),
		Calls:       slices.Collect(lt.ExecuteTemplateCalls()),
	}
}

func templates(variable string, lt *check.Templates, ts *template.Template) muxt.Templates {
	return muxt.Templates{
		Variable:  variable,
		Set:       ts,
		Functions: lt.CollectedFunctions(),
		NamePosition: func(templateName string) (token.Position, bool) {
			d, found := lt.FindDefinition(templateName)
			if !found || !d.TemplateName.IsValid() {
				return token.Position{}, false
			}
			// The span includes the quotes; the name starts one byte in.
			pos := d.TemplateName.Position
			pos.Column++
			pos.Offset++
			return pos, true
		},
	}
}

func findPackageTypes(pl []*packages.Package, path string) (*types.Package, bool) {
	for _, pkg := range pl {
		if pkg.Types != nil && pkg.Types.Path() == path {
			return pkg.Types, true
		}
	}
	for _, pkg := range pl {
		if pkg.Types == nil {
			continue
		}
		if p, ok := muxt.SearchImports(pkg.Types, path); ok {
			return p, true
		}
	}
	return nil, false
}

// AnalysisSource is Source for the commands that type check templates:
// the package at wd and each templates variable, in order, with what
// checking it needs.
func AnalysisSource(wd string, pl []*packages.Package, variables []string) (muxt.Package, []analysis.Templates, error) {
	pkg, ok := PackageAtFilepath(pl, wd)
	if !ok {
		return muxt.Package{}, nil, NoPackageError(wd, pl)
	}
	templates := make([]analysis.Templates, 0, len(variables))
	for _, variable := range variables {
		templates = append(templates, AnalysisTemplates(pkg, variable))
	}
	return Package(pkg, pl), templates, nil
}
