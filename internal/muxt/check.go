package muxt

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"path/filepath"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/source"
)

func Check(wd string, log *log.Logger, config RoutesFileConfiguration) error {
	config = config.applyDefaults()
	if !token.IsIdentifier(config.PackageName) {
		return fmt.Errorf("package name %q is not an identifier", config.PackageName)
	}

	patterns := []string{
		wd, "encoding", "fmt", "net/http",
	}

	if config.ReceiverPackage != "" {
		patterns = append(patterns, config.ReceiverPackage)
	}

	fileSet := token.NewFileSet()

	pl, err := packages.Load(&packages.Config{
		Fset: fileSet,
		Mode: packages.NeedModule | packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles,
		Dir:  wd,
	}, patterns...)
	if err != nil {
		return err
	}

	file, err := source.NewFile(filepath.Join(wd, config.OutputFileName), fileSet, pl)
	if err != nil {
		return err
	}
	routesPkg := file.OutputPackage()

	ts, fm, err := source.Templates(wd, config.TemplatesVariable, routesPkg)
	if err != nil {
		return err
	}
	fns := check.DefaultFunctions(routesPkg.Types)
	fns = fns.Add(check.Functions(fm))

	global := check.NewGlobal(routesPkg.Types, routesPkg.Fset, newForrest(ts), fns)

	var errs []error
	for _, file := range routesPkg.Syntax {
		for node := range ast.Preorder(file) {
			templateName, dataType, ok := source.ExecuteTemplateArguments(node, routesPkg.TypesInfo, config.TemplatesVariable)
			if !ok {
				continue
			}
			log.Println("checking endpoint", templateName)
			ts2 := ts.Lookup(templateName)
			if ts2 == nil {
				return fmt.Errorf("template %q not found in %q (try running generate again)", templateName, config.TemplatesVariable)
			}
			tree := ts2.Tree
			if err := check.Execute(global, tree, dataType); err != nil {
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
