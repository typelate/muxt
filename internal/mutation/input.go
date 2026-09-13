package mutation

import (
	"text/template/parse"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/source"
)

// input is what a run plans from: the package its templates variables are
// declared in, and the directory reported paths are relative to.
//
// loadInput builds one with the go command. Everything after that --
// traversal, enumeration, the --diff comparison -- reads only this, so a
// test can plan from a package built in memory.
type input struct {
	dir string
	pkg source.Package
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
	return inputFrom(dir, pl, config.TemplatesVariables)
}

// inputFrom reads the templates variables from the package in dir among
// pl.
func inputFrom(dir string, pl []*packages.Package, variables []string) (input, error) {
	pkg, err := load.Package(dir, pl, variables)
	if err != nil {
		return input{}, err
	}
	return input{dir: dir, pkg: pkg}, nil
}

// checked is one templates variable with the checker built for it, which
// traversal and the validity check share.
type checked struct {
	source.Variable
	pkg    source.Package
	global *check.Global
}

func newChecked(pkg source.Package, variable source.Variable) *checked {
	trees := check.FindTreeFunc(func(name string) (*parse.Tree, bool) {
		t := variable.Set.Lookup(name)
		if t == nil || t.Tree == nil {
			return nil, false
		}
		return t.Tree, true
	})
	return &checked{
		Variable: variable,
		pkg:      pkg,
		global:   check.NewGlobal(pkg.Types, pkg.Fset, trees, check.Functions(variable.Functions)),
	}
}
