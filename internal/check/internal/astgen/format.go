package astgen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/printer"
	"go/token"
)

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
