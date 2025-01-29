package muxt

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/crhntr/muxt/internal/source"
	"github.com/crhntr/muxt/internal/templatetype"
)

func CheckTemplates(wd string, log *log.Logger, config RoutesFileConfiguration) error {
	config = config.applyDefaults()
	if !token.IsIdentifier(config.PackageName) {
		return fmt.Errorf("package name %q is not an identifier", config.PackageName)
	}
	imports := source.NewImports(&ast.GenDecl{Tok: token.IMPORT})
	if config.ReceiverType == "" {
		return fmt.Errorf("receiver-type is required")
	}

	patterns := []string{
		wd, "encoding", "fmt", "net/http",
	}

	if config.ReceiverPackage != "" {
		patterns = append(patterns, config.ReceiverPackage)
	}

	pl, err := packages.Load(&packages.Config{
		Fset: imports.FileSet(),
		Mode: packages.NeedModule | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles,
		Dir:  wd,
	}, patterns...)
	if err != nil {
		return err
	}
	imports.AddPackages(pl...)

	routesPkg, ok := imports.PackageAtFilepath(wd)
	if !ok {
		return fmt.Errorf("could not find package in working directory %q", wd)
	}

	ts, fm, err := source.Templates(wd, config.TemplatesVariable, routesPkg)
	if err != nil {
		return err
	}
	templates, err := Templates(ts)
	if err != nil {
		return err
	}

	receiverPkgPath := cmp.Or(config.ReceiverPackage, config.PackagePath, routesPkg.PkgPath)
	receiverPkg, ok := imports.Package(receiverPkgPath)
	if !ok {
		return fmt.Errorf("could not determine receiver package %s", receiverPkgPath)
	}
	obj := receiverPkg.Types.Scope().Lookup(config.ReceiverType)
	if obj == nil {
		return fmt.Errorf("could not find receiver type %s in %s", config.ReceiverType, receiverPkgPath)
	}
	receiver, ok := obj.Type().(*types.Named)
	if !ok {
		return fmt.Errorf("expected receiver %s to be a named type", config.ReceiverType)
	}
	if receiver == nil {
		return fmt.Errorf("could not find receiver %s in %s", config.ReceiverType, receiverPkgPath)
	}

	var errs []error

	for _, t := range templates {
		var (
			dataVar    types.Type
			dataVarPkg *types.Package
		)

		log.Println("checking endpoint", t.template.Name())

		if t.fun != nil {
			name := t.fun.Name
			dataVarPkg = receiver.Obj().Pkg()
			methodObj, _, _ := types.LookupFieldOrMethod(receiver, true, dataVarPkg, name)
			if methodObj == nil {
				o, ok := packageScopeFunc(receiver.Obj().Pkg(), t.fun)
				if !ok {
					return fmt.Errorf("failed to generate method %s", t.fun.Name)
				}
				methodObj = o
			}
			sig := methodObj.Type().(*types.Signature)
			if sig.Results().Len() == 0 {
				return fmt.Errorf("method for pattern %q has no results it should have one or two", t.name)
			}
			dataVar = sig.Results().At(0).Type()
			if types.Identical(dataVar, types.Universe.Lookup("any").Type()) {
				log.Printf("\troute method returns type any\n\n\t%s\n", sig)
				continue
			}
		} else {
			netHTTP, ok := imports.Types("net/http")
			if !ok {
				return fmt.Errorf("net/http package not loaded")
			}
			dataVar = types.NewPointer(netHTTP.Scope().Lookup("Request").Type())
			dataVarPkg = netHTTP
		}
		if dataVar == nil {
			return fmt.Errorf("failed to find data var type for template %q", t.template.Name())
		}

		log.Println("\tfor data type", dataVar.String())
		log.Println()

		fns := templatetype.DefaultFunctions(routesPkg.Types)
		fns = fns.Add(templatetype.Functions(fm))

		if err := templatetype.Check(t.template.Tree, dataVar, dataVarPkg, routesPkg.Fset, newForrest(ts), fns); err != nil {
			log.Println("ERROR", templatetype.Check(t.template.Tree, dataVar, dataVarPkg, routesPkg.Fset, newForrest(ts), fns))
			log.Println()
			errs = append(errs, err)
		}
	}

	if len(errs) == 1 {
		log.Printf("1 error")
		return errs[0]
	} else if len(errs) > 0 {
		log.Printf("%d errors\n", len(errs))
		for i, err := range errs {
			fmt.Printf("- %d: %s\n", i+1, err.Error())
		}
		return errors.Join(errs...)
	}

	log.Println("OK")
	return nil
}
