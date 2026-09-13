package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"

	"golang.org/x/tools/imports"
)

// formatFile prints a generated file and formats it as goimports does,
// grouping the standard library's imports apart from the rest.
//
// The imports are the file's own: each file registers the packages its
// declarations reference as it builds them, so there is nothing to add or
// remove, and goimports is only asked to format.
func formatFile(filePath string, f *ast.File) (string, error) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), f); err != nil {
		return "", fmt.Errorf("formatting error: %v", err)
	}
	out, err := imports.Process(filePath, buf.Bytes(), &imports.Options{
		Fragment:   true,
		AllErrors:  true,
		Comments:   true,
		FormatOnly: true,
	})
	if err != nil {
		return "", fmt.Errorf("formatting error: %v", err)
	}
	return string(bytes.ReplaceAll(out, []byte("\n}\nfunc "), []byte("\n}\n\nfunc "))), nil
}
