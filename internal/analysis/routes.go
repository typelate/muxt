package analysis

import (
	"cmp"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
	"github.com/typelate/muxt/internal/muxt"
)

type DefinitionsConfiguration struct {
	Verbose           bool
	ReceiverPackage   string
	PackageName       string
	PackagePath       string
	ReceiverType      string
	TemplatesVariable string
}

type Function struct {
	Name      string
	Signature string
}
type Definition struct {
	String    string
	Separator string
	Source    string
}

type ReceiverMethod struct {
	Name      string
	Signature string
}

type Definitions struct {
	Functions       []Function
	Definitions     []Definition
	Receiver        string
	ReceiverMethods []ReceiverMethod
}

func NewRoutes(config DefinitionsConfiguration, w io.Writer, wd string, _ *token.FileSet, pl []*packages.Package) error {
	routesPkg, ok := asteval.PackageAtFilepath(pl, wd)
	if !ok {
		return fmt.Errorf("package %q not found", config.ReceiverPackage)
	}

	config.PackagePath = routesPkg.PkgPath
	config.PackageName = routesPkg.Name
	var receiver *types.Named
	if config.ReceiverType != "" {
		receiverPkgPath := cmp.Or(config.ReceiverPackage, config.PackagePath)
		receiverPkg, ok := asteval.PackageWithPath(pl, receiverPkgPath)
		if !ok {
			return fmt.Errorf("could not find receiver package %s", receiverPkgPath)
		}
		obj := receiverPkg.Types.Scope().Lookup(config.ReceiverType)
		if config.ReceiverType != "" && obj == nil {
			return fmt.Errorf("could not find receiver type %s in %s", config.ReceiverType, receiverPkg.PkgPath)
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			return fmt.Errorf("expected receiver %s to be a named type", config.ReceiverType)
		}
		receiver = named
	} else {
		receiver = types.NewNamed(types.NewTypeName(0, routesPkg.Types, "Receiver", nil), types.NewStruct(nil, nil), nil)
	}

	ts, functions, err := asteval.Templates(wd, config.TemplatesVariable, routesPkg)
	if err != nil {
		return err
	}
	definitions, err := muxt.Definitions(ts)
	if err != nil {
		return err
	}
	return writeRoutesList(w, functions, definitions, receiver)
}

func writeRoutesList(w io.Writer, functions asteval.TemplateFunctions, defs []muxt.Definition, receiver *types.Named) error {
	var funcList []Function
	names := slices.Collect(maps.Keys(functions))
	for _, name := range names {
		s := strings.TrimPrefix(functions[name].String(), "func")
		funcList = append(funcList, Function{Name: name, Signature: s})
	}

	var defList []Definition
	for _, def := range defs {
		src := def.Template().Tree.Root.String()
		defList = append(defList, Definition{
			String:    def.String(),
			Separator: strings.Repeat("=", 40),
			Source:    src,
		})
	}

	var methods []ReceiverMethod
	for i := 0; i < receiver.NumMethods(); i++ {
		m := receiver.Method(i)
		methods = append(methods, ReceiverMethod{
			Name:      m.Name(),
			Signature: strings.TrimPrefix(m.Signature().String(), "func"),
		})
	}

	return templates.ExecuteTemplate(w, "routes.txt.template", Definitions{
		Functions:       funcList,
		Definitions:     defList,
		Receiver:        receiver.String(),
		ReceiverMethods: methods,
	})
}
