package generate

import (
	"fmt"
	"strings"

	"github.com/ettle/strcase"
)

// Namespace returns the name a package's routes have on the hub's paths type.
func Namespace(importPath, hubImportPath string) (string, error) {
	rest, ok := strings.CutPrefix(importPath, hubImportPath+"/")
	if !ok {
		return "", fmt.Errorf("package %q is not below hub %q", importPath, hubImportPath)
	}
	var sb strings.Builder
	for _, elem := range strings.Split(rest, "/") {
		sb.WriteString(strcase.ToGoPascal(elem))
	}
	return sb.String(), nil
}
