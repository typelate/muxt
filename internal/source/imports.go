package source

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"path"
	"slices"
	"strconv"
	"strings"
)

type Imports struct {
	*ast.GenDecl
	fileSet       *token.FileSet
	types         map[string]*types.Package
	files         map[string]*ast.File
	outputPackage string
}

func NewImports(decl *ast.GenDecl) *Imports {
	if decl != nil {
		if got := decl.Tok; got != token.IMPORT {
			log.Panicf("expected decl to have token.IMPORT Tok got %s", got)
		}
	}
	return &Imports{GenDecl: decl, types: make(map[string]*types.Package), files: make(map[string]*ast.File)}
}

func (imports *Imports) AddPackages(p *types.Package) {
	recursivelyRegisterPackages(imports.types, p)
}

func (imports *Imports) FileSet() *token.FileSet {
	if imports.fileSet == nil {
		imports.fileSet = token.NewFileSet()
	}
	return imports.fileSet
}

func (imports *Imports) SetOutputPackage(pkgPath string) {
	imports.outputPackage = pkgPath
}

func (imports *Imports) OutputPackage() string {
	return imports.outputPackage
}

func (imports *Imports) SyntaxFile(pos token.Pos) (*ast.File, *token.FileSet, error) {
	position := imports.FileSet().Position(pos)
	fSet := token.NewFileSet()
	file, err := parser.ParseFile(fSet, position.Filename, nil, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
	return file, fSet, err
}

func (imports *Imports) FieldTag(pos token.Pos) (*ast.Field, error) {
	file, fileSet, err := imports.SyntaxFile(pos)
	if err != nil {
		return nil, err
	}
	position := imports.fileSet.Position(pos)
	for _, d := range file.Decls {
		switch decl := d.(type) {
		case *ast.GenDecl:
			for _, s := range decl.Specs {
				switch spec := s.(type) {
				case *ast.TypeSpec:
					tp, ok := spec.Type.(*ast.StructType)
					if !ok {
						continue
					}

					for _, field := range tp.Fields.List {
						for _, name := range field.Names {
							p := fileSet.Position(name.Pos())
							if p != position {
								continue
							}
							return field, nil
						}
					}
				}
			}
		}

	}
	return nil, fmt.Errorf("failed to find field")
}

func (imports *Imports) Types(pkgPath string) (*types.Package, bool) {
	p, ok := imports.types[pkgPath]
	return p, ok
}

func recursivelyRegisterPackages(set map[string]*types.Package, pkg *types.Package) {
	if pkg == nil {
		return
	}
	set[pkg.Path()] = pkg
	for _, p := range pkg.Imports() {
		recursivelyRegisterPackages(set, p)
	}
}

func (imports *Imports) Add(pkgIdent, pkgPath string) string {
	if imports.GenDecl == nil {
		imports.GenDecl = new(ast.GenDecl)
		imports.GenDecl.Tok = token.IMPORT
	}
	if pkgIdent == "" {
		pkgIdent = path.Base(pkgPath)
	}
	if pkgPath != imports.outputPackage {
		for _, s := range imports.GenDecl.Specs {
			spec := s.(*ast.ImportSpec)
			pp, _ := strconv.Unquote(spec.Path.Value)
			if pp == pkgPath {
				if spec.Name != nil && spec.Name.Name != "" && spec.Name.Name != pkgIdent {
					return spec.Name.Name
				}
				return path.Base(pp)
			}
		}
		var pi *ast.Ident
		if path.Base(pkgPath) != pkgIdent {
			pi = Ident(pkgIdent)
		}
		imports.GenDecl.Specs = append(imports.GenDecl.Specs, &ast.ImportSpec{
			Path: String(pkgPath),
			Name: pi,
		})
		slices.SortFunc(imports.GenDecl.Specs, func(a, b ast.Spec) int {
			return strings.Compare(a.(*ast.ImportSpec).Path.Value, b.(*ast.ImportSpec).Path.Value)
		})
	}
	return pkgIdent
}

func (imports *Imports) Ident(pkgPath string) string {
	if imports != nil && imports.GenDecl != nil {
		for _, s := range imports.GenDecl.Specs {
			spec := s.(*ast.ImportSpec)
			pp, _ := strconv.Unquote(spec.Path.Value)
			if pp == pkgPath {
				if spec.Name != nil && spec.Name.Name != "" {
					return spec.Name.Name
				}
				return path.Base(pp)
			}
		}
	}
	return path.Base(pkgPath)
}

func (imports *Imports) Call(pkgName, pkgPath, funcIdent string, args []ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(imports.Add(pkgName, pkgPath)),
			Sel: ast.NewIdent(funcIdent),
		},
		Args: args,
	}
}

func (imports *Imports) ImportSpecs() []*ast.ImportSpec {
	result := make([]*ast.ImportSpec, 0, len(imports.GenDecl.Specs))
	for _, spec := range imports.GenDecl.Specs {
		result = append(result, spec.(*ast.ImportSpec))
	}
	slices.SortFunc(result, func(a, b *ast.ImportSpec) int { return strings.Compare(a.Path.Value, b.Path.Value) })
	return slices.CompactFunc(result, func(a, b *ast.ImportSpec) bool { return a.Path.Value == b.Path.Value })
}

func (imports *Imports) SortImports() {
	sorted := imports.GenDecl.Specs[:0]
	for _, spec := range imports.ImportSpecs() {
		sorted = append(sorted, spec)
	}
	imports.GenDecl.Specs = sorted
}

func (imports *Imports) AddNetHTTP() string      { return imports.Add("", "net/http") }
func (imports *Imports) AddHTMLTemplate() string { return imports.Add("", "html/template") }
func (imports *Imports) AddContext() string      { return imports.Add("", "context") }

func (imports *Imports) HTTPErrorCall(response, message ast.Expr, code int) *ast.CallExpr {
	return imports.Call("", "net/http", "Error", []ast.Expr{
		response,
		message,
		HTTPStatusCode(imports, code),
	})
}

func (imports *Imports) StrconvAtoiCall(expr ast.Expr) *ast.CallExpr {
	return imports.Call("", "strconv", "Atoi", []ast.Expr{expr})
}

func (imports *Imports) StrconvParseIntCall(expr ast.Expr, base, size int) *ast.CallExpr {
	return imports.Call("", "strconv", "ParseInt", []ast.Expr{expr, Int(base), Int(size)})
}

func (imports *Imports) StrconvParseUintCall(expr ast.Expr, base, size int) *ast.CallExpr {
	return imports.Call("", "strconv", "ParseUint", []ast.Expr{expr, Int(base), Int(size)})
}

func (imports *Imports) StrconvParseFloatCall(expr ast.Expr, size int) *ast.CallExpr {
	return imports.Call("", "strconv", "ParseFloat", []ast.Expr{expr, Int(size)})
}

func (imports *Imports) StrconvParseBoolCall(expr ast.Expr) *ast.CallExpr {
	return imports.Call("", "strconv", "ParseBool", []ast.Expr{expr})
}

func (imports *Imports) TimeParseCall(layout string, expr ast.Expr) *ast.CallExpr {
	return imports.Call("", "time", "Parse", []ast.Expr{String(layout), expr})
}
