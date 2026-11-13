package load

import (
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

// Loaded is the packages of one go command run. Its fields are unexported so
// the go/packages values stay inside this package: a caller asks it questions
// and gets plain data back. The zero value is a run that found no packages.
type Loaded struct {
	pl []*packages.Package
}

// InDirectories runs the go command once in root for dirs and patterns, so a
// run over several package directories pays for one load.
func InDirectories(root string, dirs, patterns []string) (*Loaded, error) {
	_, pl, err := PackagesInDirectories(root, nil, dirs, patterns...)
	if err != nil {
		return nil, err
	}
	return &Loaded{pl: pl}, nil
}

// HasParseErrors reports whether the loader recovered from a syntax error in
// any package.
func (l *Loaded) HasParseErrors() bool {
	return len(ParseErrors(l.pl)) > 0
}

// CheckLoaded returns why the package in dir did not load, or nil.
func (l *Loaded) CheckLoaded(dir string) error {
	return CheckLoaded(dir, l.pl)
}

// Package returns the package in dir with its templates variables.
func (l *Loaded) Package(dir string, variables []string) (source.Package, error) {
	return Package(dir, l.pl, variables)
}

// PackageWithReceiver is Package with the receiver type its routes call.
func (l *Loaded) PackageWithReceiver(dir string, name ReceiverName, variables []string) (source.Package, *types.Named, error) {
	return PackageWithReceiver(dir, l.pl, name, variables)
}

// Receiver returns the named receiver type, looked up from the package in dir
// when name.Package is empty.
func (l *Loaded) Receiver(dir string, name ReceiverName) (*types.Named, error) {
	return Receiver(dir, l.pl, name)
}

// CheckFileRead reports a *UnreadFileError when no templates variable of the
// package in dir reads the file name.
func (l *Loaded) CheckFileRead(dir string, variables []string, name string) error {
	return CheckFileRead(dir, l.pl, variables, name)
}

// StandardLibrary answers muxt's questions about the standard library from
// what this run loaded.
func (l *Loaded) StandardLibrary() muxt.Checker {
	return StandardLibrary(l.pl)
}

// Imports returns, for each of dirs, the indexes of the dirs its package
// imports, directly or not. A directory with no package loaded imports
// nothing and is imported by nothing.
func (l *Loaded) Imports(dirs []string) ImportGraph {
	graph := make(ImportGraph, len(dirs))
	for i := range dirs {
		for j := range dirs {
			if i != j && Imports(l.pl, dirs[i], dirs[j]) {
				graph[i] = append(graph[i], j)
			}
		}
	}
	return graph
}
