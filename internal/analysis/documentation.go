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
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxt"
)

func Documentation(w io.Writer, wd string, config generate.RoutesFileConfiguration) error {
	if !token.IsIdentifier(config.PackageName) {
		return fmt.Errorf("package name %q is not an identifier", config.PackageName)
	}

	patterns := []string{wd, "net/http"}
	if config.ReceiverPackage != "" {
		patterns = append(patterns, config.ReceiverPackage)
	}

	fileSet := token.NewFileSet()
	pl, err := packages.Load(&packages.Config{
		Fset: fileSet,
		Mode: packages.NeedModule | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles,
		Dir:  wd,
	}, patterns...)
	if err != nil {
		return err
	}

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
	templates, err := muxt.Definitions(ts)
	if err != nil {
		return err
	}

	writeOutput(w, functions, templates, receiver)

	return nil
}

func writeOutput(w io.Writer, functions asteval.TemplateFunctions, defs []muxt.Definition, receiver *types.Named) {
	_, _ = fmt.Fprintf(w, "functions:\n")
	names := slices.Collect(maps.Keys(functions))
	for _, name := range names {
		s := strings.TrimPrefix(functions[name].String(), "func")
		_, _ = fmt.Fprintf(w, "  - func %s%s\n", name, s)
	}

	_, _ = fmt.Fprintf(w, "\nTemplate Routes:\n\n")
	for _, def := range defs {
		_, _ = fmt.Fprintf(w, "%s\n", def.String())

		const prefix = "<!DOCTYPE"
		if src := def.Template().Tree.Root.String(); len(src) >= len(prefix) && strings.EqualFold(src[:len(prefix)], "<!DOCTYPE") {
			_, _ = fmt.Fprintf(w, "%s\n%s\n%s\n\n\n", strings.Repeat("=", 40), src, strings.Repeat("-", 40))
		} else {
			_, _ = fmt.Fprintf(w, "%s\n%s\n%s\n\n\n", strings.Repeat("=", 40), src, strings.Repeat("-", 40))
		}
	}

	_, _ = fmt.Fprintf(w, "\nReceiver Type: %s\n", receiver.String())
	if receiver.NumMethods() > 0 {
		_, _ = fmt.Fprintf(w, "\nReceiver Methods:\n")
	}
	for i := 0; i < receiver.NumMethods(); i++ {
		m := receiver.Method(i)
		_, _ = fmt.Fprintf(w, "  - func (%s) %s%s\n", receiver.String(), m.Name(), strings.TrimPrefix(m.Signature().String(), "func"))
	}
}
