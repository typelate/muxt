package source

import (
	"go/ast"
	"path"
	"slices"
	"strconv"
	"strings"
)

const goEmbedCommentPrefix = "//go:embed"

type GoFiles []*ast.File

func (files GoFiles) ImportReceiverMethods(tp, name string) (*ast.FuncType, []*ast.ImportSpec, bool) {
	for file, fun := range IterateFunctions(files) {
		if fun.Name.Name != name {
			continue
		}
		if fun.Recv == nil || len(fun.Recv.List) != 1 {
			continue
		}
		if rn, ok := fun.Recv.List[0].Type.(*ast.Ident); !ok || rn.Name != tp {
			continue
		}
		var imports []*ast.ImportSpec
		for _, n := range fun.Type.Params.List {
			imports = append(imports, getPackages(file, n.Type)...)
			slices.SortFunc(imports, func(a, b *ast.ImportSpec) int { return strings.Compare(a.Path.Value, b.Path.Value) })
			imports = slices.CompactFunc(imports, func(a, b *ast.ImportSpec) bool { return a.Path.Value == b.Path.Value })
		}
		return fun.Type, imports, true
	}
	return nil, nil, false
}

func getPackages(file *ast.File, x ast.Expr) []*ast.ImportSpec {
	var specs []*ast.ImportSpec
	ast.Inspect(x, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok {
			return true // should never happen
		}
		for _, in := range file.Imports {
			p, _ := strconv.Unquote(in.Path.Value)
			var importName string
			if in.Name != nil {
				importName = in.Name.Name
			} else {
				importName = path.Base(p)
			}
			if importName != pkg.Name {
				continue
			}
			specs = append(specs, in)
			break
		}
		return false
	})
	return specs
}
