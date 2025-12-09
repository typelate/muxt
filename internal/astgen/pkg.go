package astgen

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"path"
	"slices"
	"strconv"
)

type TypeFormatter struct {
	OutPackage string
	Imports    map[string]string
	Idents     []string
}

func NewTypeFormatter(outputPkgPath string) *TypeFormatter {
	return &TypeFormatter{
		OutPackage: outputPkgPath,
		Imports:    make(map[string]string),
	}
}

func (tf *TypeFormatter) Qualifier(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	pth := pkg.Path()
	if pth == tf.OutPackage {
		return ""
	}
	name, ok := tf.Imports[pth]
	if ok {
		return name
	}
	name = pkg.Name()
	for i := 1; i < 100; i++ {
		if slices.Contains(tf.Idents, name) {
			name = pkg.Name() + strconv.Itoa(i)
			continue
		}
		break
	}
	tf.Imports[pth] = name
	return name
}

func (tf *TypeFormatter) GenDecl() *ast.GenDecl {
	decl := &ast.GenDecl{
		Tok: token.IMPORT,
	}

	packages := slices.Collect(maps.Keys(tf.Imports))
	slices.Sort(packages)
	for _, pkg := range packages {
		end := path.Base(pkg)
		ident := tf.Imports[pkg]
		spec := &ast.ImportSpec{
			Path: String(pkg),
		}
		if end != ident {
			spec.Name = ast.NewIdent(ident)
		}
		decl.Specs = append(decl.Specs, spec)
	}

	return decl
}
