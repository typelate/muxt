package mutation

import (
	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/templateset"
)

// input is what a run plans from: the package its templates variables are
// declared in, and each variable as type checking reads it.
//
// loadInput builds one with the go command. Everything after that --
// traversal, enumeration, the --diff comparison -- reads only this, so a
// test can plan from a package built in memory.
type input struct {
	// dir is the directory the package was loaded from. Reported paths
	// and positions are relative to it.
	dir string

	pkg       muxt.Package
	variables []templateset.Variable
}

// loadInput loads the package in dir, with its test files when the
// configuration includes test callers. env is the environment the go
// command runs in, nil for the process's own.
func loadInput(dir string, config Configuration, env []string) (input, error) {
	var (
		pl  []*packages.Package
		err error
	)
	if config.IncludeTests {
		pl, err = load.PackagesWithTests(dir, env)
	} else {
		_, pl, err = load.PackagesWithEnv(dir, env)
	}
	if err != nil {
		return input{}, err
	}
	return inputFrom(dir, pl, config.TemplatesVariables), nil
}

// inputFrom reads the templates variables from the package in dir among
// pl. When there is no package there, each variable carries that error,
// so a run reports it where it reaches the first variable, as it would
// have loading each one on demand.
func inputFrom(dir string, pl []*packages.Package, variables []string) input {
	pkg, sets, err := load.TemplateSets(dir, pl, variables)
	if err != nil {
		sets = make([]templateset.Variable, 0, len(variables))
		for _, variable := range variables {
			sets = append(sets, templateset.Variable{Templates: muxt.Templates{Variable: variable, Err: err}})
		}
	}
	return input{dir: dir, pkg: pkg, variables: sets}
}

// checked is one templates variable with the checker built for it, which
// traversal and the validity check share.
type checked struct {
	templateset.Variable
	pkg    muxt.Package
	global *check.Global
}

func newChecked(pkg muxt.Package, variable templateset.Variable) *checked {
	return &checked{Variable: variable, pkg: pkg, global: variable.Global(pkg)}
}
