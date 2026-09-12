package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"

	"golang.org/x/tools/imports"
)

// formatFile prints a generated file and runs goimports over it, which
// drops any import a handler registered but did not end up using.
func formatFile(filePath string, f *ast.File) (string, error) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), f); err != nil {
		return "", fmt.Errorf("formatting error: %v", err)
	}
	out, err := imports.Process(filePath, buf.Bytes(), &imports.Options{
		Fragment:  true,
		AllErrors: true,
		Comments:  true,
	})
	if err != nil {
		return "", fmt.Errorf("formatting error: %v", err)
	}
	return string(bytes.ReplaceAll(out, []byte("\n}\nfunc "), []byte("\n}\n\nfunc "))), nil
}
