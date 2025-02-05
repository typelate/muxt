package muxt

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
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
		Mode: packages.NeedModule | packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles,
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
	fns := templatetype.DefaultFunctions(routesPkg.Types)
	fns = fns.Add(templatetype.Functions(fm))

	var errs []error
	for _, file := range routesPkg.Syntax {
		for node := range ast.Preorder(file) {
			templateName, dataType, ok := source.ExecuteTemplateArguments(node, routesPkg.TypesInfo, config.TemplatesVariable)
			if !ok {
				continue
			}
			log.Println("checking endpoint", templateName)
			tree := ts.Lookup(templateName).Tree
			if err := templatetype.Check(tree, dataType, routesPkg.Types, routesPkg.Fset, newForrest(ts), fns); err != nil {
				log.Println("ERROR", err)
				log.Println()
				errs = append(errs, err)
			}
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
