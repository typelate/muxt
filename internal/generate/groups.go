package generate

import (
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
	"github.com/typelate/muxt/internal/muxt"
)

type templateGroups struct {
	byFile map[string][]muxt.Definition
	noFile []muxt.Definition
	all    []muxt.Definition
}

func groupTemplates(wd string, config RoutesFileConfiguration, routesPkg *packages.Package) (templateGroups, error) {
	result := templateGroups{
		byFile: make(map[string][]muxt.Definition),
	}
	for _, tv := range config.TemplatesVariables {
		ts, _, err := asteval.Templates(wd, tv, routesPkg)
		if err != nil {
			return result, err
		}

		defs, err := muxt.Definitions(ts, tv)
		if err != nil {
			return result, err
		}

		for _, d := range defs {
			key := d.SourceFile()
			result.byFile[key] = append(result.byFile[key], d)
		}
		result.all = append(result.all, defs...)
	}

	if err := muxt.CheckForDuplicatePatterns(result.all); err != nil {
		return result, err
	}

	result.noFile = result.byFile[""]
	delete(result.byFile, "")

	for sourceFile := range result.byFile {
		baseName := filepath.Base(sourceFile)
		if strings.ContainsAny(baseName, " /\\()") {
			result.noFile = append(result.noFile, result.byFile[sourceFile]...)
			delete(result.byFile, sourceFile)
		}
	}
	return result, nil
}
