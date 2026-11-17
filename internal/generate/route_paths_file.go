package generate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ettle/strcase"
)

// CheckNamespaces reports the first two packages that would share a namespace.
func CheckNamespaces(importPaths []string, hubImportPath string) error {
	sorted := slices.Clone(importPaths)
	slices.Sort(sorted)
	seen := make(map[string]string, len(sorted))
	for _, importPath := range sorted {
		namespace, err := Namespace(importPath, hubImportPath)
		if err != nil {
			return err
		}
		if first, ok := seen[namespace]; ok {
			return &SameNamespaceError{Namespace: namespace, ImportPaths: [2]string{first, importPath}}
		}
		seen[namespace] = importPath
	}
	return nil
}

// SameNamespaceError is two packages whose routes would get one namespace.
type SameNamespaceError struct {
	Namespace   string
	ImportPaths [2]string // sorted
}

func (e *SameNamespaceError) Error() string {
	return fmt.Sprintf("packages %q and %q both get the route paths namespace %q; rename one of them",
		e.ImportPaths[0], e.ImportPaths[1], e.Namespace)
}

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
