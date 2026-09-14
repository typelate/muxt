package load

import (
	"fmt"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/muxt"
)

// StandardLibrary answers route resolution's questions about the standard
// library from the packages a load reached: those in pl and every package
// they import. It is the one muxt.Checker backed by the official standard
// library, whichever version the go command loaded.
func StandardLibrary(pl []*packages.Package) muxt.Checker {
	return standardLibrary(indexImports(pl))
}

// standardLibrary is a muxt.Checker over packages indexed by import path.
type standardLibrary map[string]*types.Package

// scopeTypes names, for each reserved argument identifier, the standard
// library type it binds to and whether it binds a pointer to that type.
var scopeTypes = map[string]struct {
	path, name string
	pointer    bool
}{
	muxt.TemplateNameScopeIdentifierHTTPRequest:  {"net/http", "Request", true},
	muxt.TemplateNameScopeIdentifierHTTPResponse: {"net/http", "ResponseWriter", false},
	muxt.TemplateNameScopeIdentifierContext:      {"context", "Context", false},
	muxt.TemplateNameScopeIdentifierForm:         {"net/url", "Values", false},
	muxt.TemplateNameScopeIdentifierMultipart:    {"mime/multipart", "Form", true},
	muxt.TemplateNameScopeIdentifierRequestBody:  {"io", "Reader", false},
}

func (std standardLibrary) ScopeType(identifier string) (types.Type, error) {
	scope, ok := scopeTypes[identifier]
	if !ok {
		return nil, fmt.Errorf("%s is not a reserved argument identifier", identifier)
	}
	return std.lookup(scope.path, scope.name, scope.pointer)
}

func (std standardLibrary) FileHeader() (types.Type, error) {
	return std.lookup("mime/multipart", "FileHeader", true)
}

func (std standardLibrary) RawJSON() (types.Type, error) {
	return std.lookup("encoding/json", "RawMessage", false)
}

func (std standardLibrary) TextUnmarshaler(tp types.Type) bool {
	return std.implements(types.NewPointer(tp), "TextUnmarshaler")
}

func (std standardLibrary) TextMarshaler(tp types.Type) bool {
	return std.implements(tp, "TextMarshaler")
}

func (std standardLibrary) implements(tp types.Type, encodingInterface string) bool {
	iface, err := std.lookup("encoding", encodingInterface, false)
	if err != nil {
		return false
	}
	return types.Implements(tp, iface.Underlying().(*types.Interface))
}

func (std standardLibrary) lookup(path, name string, pointer bool) (types.Type, error) {
	pkg, ok := std[path]
	if !ok {
		return nil, fmt.Errorf("could not find package %q for %s", path, name)
	}
	obj := pkg.Scope().Lookup(name)
	if obj == nil {
		return nil, fmt.Errorf("package %q declares no %s", path, name)
	}
	tp := obj.Type()
	if pointer {
		tp = types.NewPointer(tp)
	}
	return tp, nil
}

// indexImports indexes, by import path, every package in pl and every
// package they import. A path loaded more than once -- a package and its
// test variant -- keeps the first in pl.
func indexImports(pl []*packages.Package) map[string]*types.Package {
	index := make(map[string]*types.Package)
	var queue []*types.Package
	for _, pkg := range pl {
		if pkg.Types == nil {
			continue
		}
		if _, seen := index[pkg.Types.Path()]; !seen {
			index[pkg.Types.Path()] = pkg.Types
			queue = append(queue, pkg.Types)
		}
	}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		for _, imported := range pkg.Imports() {
			if _, seen := index[imported.Path()]; seen {
				continue
			}
			index[imported.Path()] = imported
			queue = append(queue, imported)
		}
	}
	return index
}
