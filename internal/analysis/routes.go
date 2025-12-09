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
	Receiver        *types.Named
	ReceiverMethods []ReceiverMethod
}

func NewRoutes(config DefinitionsConfiguration, w io.Writer, wd string, _ *token.FileSet, pl []*packages.Package) error {
	pkg, ok := asteval.PackageAtFilepath(pl, wd)
	if !ok {
		return fmt.Errorf("package not found in working directory")
	}

	config.PackagePath = pkg.PkgPath
	config.PackageName = pkg.Name

	var receiver *types.Named
	if config.ReceiverType != "" {
		var err error
		receiver, err = asteval.FindType(pl, cmp.Or(config.ReceiverPackage, config.PackagePath), config.ReceiverType)
		if err != nil {
			return err
		}
	}

	ts, functions, err := asteval.Templates(wd, config.TemplatesVariable, pkg)
	if err != nil {
		return err
	}
	definitions, err := muxt.Definitions(ts)
	if err != nil {
		return err
	}

	var funcList []Function
	names := slices.Collect(maps.Keys(functions))
	for _, name := range names {
		s := strings.TrimPrefix(functions[name].String(), "func")
		funcList = append(funcList, Function{Name: name, Signature: s})
	}

	var defList []Definition
	for _, def := range definitions {
		src := def.Template().Tree.Root.String()
		defList = append(defList, Definition{
			String:    def.String(),
			Separator: strings.Repeat("=", 40),
			Source:    src,
		})
	}

	def := Definitions{
		Functions:   funcList,
		Definitions: defList,
	}

	if receiver != nil {
		def.Receiver = receiver
		for i := 0; i < receiver.NumMethods(); i++ {
			m := receiver.Method(i)
			def.ReceiverMethods = append(def.ReceiverMethods, ReceiverMethod{
				Name:      m.Name(),
				Signature: strings.TrimPrefix(m.Signature().String(), "func"),
			})
		}
	}

	return templates.ExecuteTemplate(w, "routes.txt.template", def)
}
