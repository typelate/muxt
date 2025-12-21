package astgen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/printer"
	"go/token"

	"golang.org/x/tools/imports"
)

// FormatFile formats an AST file and processes imports
func FormatFile(filePath string, f *ast.File) (string, error) {
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

// Format converts an AST node to formatted Go source code
func Format(node ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("formatting error: %v", err)
	}
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Sprintf("formatting error: %v", err)
	}
	return string(bytes.ReplaceAll(out, []byte("\n}\nfunc "), []byte("\n}\n\nfunc ")))
}
