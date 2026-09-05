package asteval

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
)

const TemplateExecuteFunc = "ExecuteTemplate"

// ExecuteTemplateArguments matches node against
// templatesVariable.ExecuteTemplate(wr, "name", data) and returns the
// template name literal and the data argument's type.
func ExecuteTemplateArguments(node ast.Node, info *types.Info, templatesVariableName string) (string, types.Type, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return "", nil, false
	}
	if len(call.Args) != 3 {
		return "", nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil, false
	}
	if sel.Sel.Name != TemplateExecuteFunc {
		return "", nil, false
	}
	templatesIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	if templatesIdent.Name != templatesVariableName {
		return "", nil, false
	}
	templateName, ok := basicLiteralString(call.Args[1])
	if !ok {
		return "", nil, false
	}
	dataVar := info.TypeOf(call.Args[2])
	return templateName, dataVar, true
}

func basicLiteralString(node ast.Node) (string, bool) {
	name, ok := node.(*ast.BasicLit)
	if !ok {
		return "", false
	}
	if name.Kind != token.STRING {
		return "", false
	}
	templateName, err := strconv.Unquote(name.Value)
	if err != nil {
		return "", false
	}
	return templateName, true
}
