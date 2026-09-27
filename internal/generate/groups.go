package generate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/typelate/muxt/internal/muxt"
)

type templateGroups struct {
	byFile map[string][]muxt.Definition
	noFile []muxt.Definition
	all    []muxt.Definition
}

func groupTemplates(config RoutesFileConfiguration, defs []muxt.Definition) (templateGroups, error) {
	result := templateGroups{
		byFile: make(map[string][]muxt.Definition),
		all:    defs,
	}
	if !config.OutputDatastar {
		for _, d := range defs {
			if d.UsesSignals() {
				return result, fmt.Errorf("the signals argument in %q requires --output-datastar; it is shorthand for unmarshalJSON(body)", d.Name())
			}
			if name, ok := d.SignalsCallback(); ok {
				return result, fmt.Errorf("the %s callback in %q requires --output-datastar; it marshals its argument as a datastar-patch-signals event", name, d.Name())
			}
		}
	}
	for _, d := range defs {
		key := d.SourceFile()
		result.byFile[key] = append(result.byFile[key], d)
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
