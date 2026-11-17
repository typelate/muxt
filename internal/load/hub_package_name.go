package load

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// HubPackageName returns the package name of the hub package in dir, whose
// import path is importPath: the package clause of the first non-test Go file
// in dir other than skip, the base name of the file a run writes there. A
// directory with no such file takes the last element of the import path.
func HubPackageName(dir, importPath, skip string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading hub package %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == skip {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.PackageClauseOnly)
		if err != nil {
			return "", fmt.Errorf("reading hub package %s: %w", dir, err)
		}
		return file.Name.Name, nil
	}
	return path.Base(importPath), nil
}
