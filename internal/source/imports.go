package source

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Imports struct {
	*ast.GenDecl
	fileSet           *token.FileSet
	typesCache        map[string]*types.Package
	files             map[string]*ast.File
	packages          []*packages.Package
	outPkg            *types.Package
	outputPackage     string
	outputPackagePath string
}

func NewImports(decl *ast.GenDecl) *Imports {
	if decl != nil {
		if got := decl.Tok; got != token.IMPORT {
			log.Panicf("expected decl to have token.IMPORT Tok got %s", got)
		}
	}
	return &Imports{GenDecl: decl, typesCache: make(map[string]*types.Package), files: make(map[string]*ast.File)}
}

func (imports *Imports) Package(path string) (*packages.Package, bool) {
	for _, pkg := range imports.packages {
		if pkg.PkgPath == path {
			return pkg, true
		}
	}
	return nil, false
}

func (imports *Imports) AddPackages(packages ...*packages.Package) {
	imports.packages = slices.Grow(imports.packages, len(packages))
	for _, pkg := range packages {
		if pkg == nil {
			continue
		}
		imports.typesCache[pkg.PkgPath] = pkg.Types
		imports.packages = append(imports.packages, pkg)
	}
}

func (imports *Imports) PackageAtFilepath(p string) (*packages.Package, bool) {
	for _, pkg := range imports.packages {
		if len(pkg.GoFiles) > 0 && filepath.Dir(pkg.GoFiles[0]) == p {
			return pkg, true
		}
	}
	return nil, false
}

func (imports *Imports) FileSet() *token.FileSet {
	if imports.fileSet == nil {
		imports.fileSet = token.NewFileSet()
	}
	return imports.fileSet
}

func (imports *Imports) SetOutputPackage(pkg *types.Package) {
	imports.outPkg = pkg
	imports.outputPackage = pkg.Path()
}

func (imports *Imports) OutputPackage() string {
	return cmp.Or(imports.outputPackage, "main")
}

func (imports *Imports) OutputPackageType() *types.Package {
	return imports.outPkg
}

func (imports *Imports) SyntaxFile(pos token.Pos) (*ast.File, *token.FileSet, error) {
	position := imports.FileSet().Position(pos)
	fSet := token.NewFileSet()
	file, err := parser.ParseFile(fSet, position.Filename, nil, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
	return file, fSet, err
}

func (imports *Imports) TypeASTExpression(tp types.Type) (ast.Expr, error) {
	s := types.TypeString(tp, func(pkg *types.Package) string {
		if pkg.Path() == imports.OutputPackage() {
			return ""
		}
		return imports.Add("", pkg.Path())
	})
	return parser.ParseExpr(s)
}

func (imports *Imports) StructField(pos token.Pos) (*ast.Field, error) {
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
	if p, ok := imports.typesCache[pkgPath]; ok {
		return p, true
	}
	for _, pkg := range imports.packages {
		if pkg.Types.Path() == pkgPath {
			p := pkg.Types
			imports.typesCache[pkgPath] = p
			return p, true
		}
	}
	for _, pkg := range imports.packages {
		if p, ok := recursivelySearchImports(pkg.Types, pkgPath); ok {
			imports.typesCache[pkgPath] = p
			return p, true
		}
	}
	return nil, false
}

func recursivelySearchImports(pt *types.Package, pkgPath string) (*types.Package, bool) {
	for _, pkg := range pt.Imports() {
		if pkg.Path() == pkgPath {
			return pkg, true
		}
	}
	for _, pkg := range pt.Imports() {
		if im, ok := recursivelySearchImports(pkg, pkgPath); ok {
			return im, true
		}
	}
	return nil, false
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
			pi = ast.NewIdent(pkgIdent)
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
func (imports *Imports) AddPath() string         { return imports.Add("", "path") }
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

func (imports *Imports) StrconvItoaCall(expr ast.Expr) *ast.CallExpr {
	return imports.Call("", "strconv", "Itoa", []ast.Expr{expr})
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

func (imports *Imports) BytesNewBuffer(expr ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(imports.Add("", "bytes")),
			Sel: ast.NewIdent("NewBuffer"),
		},
		Args: []ast.Expr{expr},
	}
}

func (imports *Imports) HTTPRequestPtr() *ast.StarExpr {
	return &ast.StarExpr{
		X: &ast.SelectorExpr{
			X:   ast.NewIdent(imports.Add("http", "net/http")),
			Sel: ast.NewIdent("Request"),
		},
	}
}

func (imports *Imports) HTTPResponseWriter() *ast.SelectorExpr {
	return &ast.SelectorExpr{
		X:   ast.NewIdent(imports.Add("http", "net/http")),
		Sel: ast.NewIdent("ResponseWriter"),
	}
}

func (imports *Imports) HTTPHeader() *ast.SelectorExpr {
	return &ast.SelectorExpr{
		X:   ast.NewIdent(imports.Add("http", "net/http")),
		Sel: ast.NewIdent("Header"),
	}
}

func (imports *Imports) StrconvParseInt8Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseIntCall(in, 10, 8)
}

func (imports *Imports) StrconvParseInt16Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseIntCall(in, 10, 16)
}

func (imports *Imports) StrconvParseInt32Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseIntCall(in, 10, 32)
}

func (imports *Imports) StrconvParseInt64Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseIntCall(in, 10, 64)
}

func (imports *Imports) StrconvParseUint0Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseUintCall(in, 10, 0)
}

func (imports *Imports) StrconvParseUint8Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseUintCall(in, 10, 8)
}

func (imports *Imports) StrconvParseUint16Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseUintCall(in, 10, 16)
}

func (imports *Imports) StrconvParseUint32Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseUintCall(in, 10, 32)
}

func (imports *Imports) StrconvParseUint64Call(in ast.Expr) *ast.CallExpr {
	return imports.StrconvParseUintCall(in, 10, 64)
}

func (imports *Imports) FormatInt(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("Itoa")},
		Args: []ast.Expr{in},
	}
}

func (imports *Imports) FormatInt8(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatInt")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

func (imports *Imports) FormatInt16(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatInt")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

func (imports *Imports) FormatInt32(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatInt")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

func (imports *Imports) FormatInt64(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatInt")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

func (imports *Imports) FormatUint(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

func (imports *Imports) FormatUint8(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

func (imports *Imports) FormatUint16(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

func (imports *Imports) FormatUint32(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

func (imports *Imports) FormatUint64(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
		Args: []ast.Expr{in, Int(10)},
	}
}

func (imports *Imports) FormatBool(in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatBool")},
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("bool"), Args: []ast.Expr{in}}},
	}
}

func (imports *Imports) Format(variable ast.Expr, kind types.BasicKind) (ast.Expr, error) {
	switch kind {
	case types.Bool, types.UntypedBool:
		return imports.FormatBool(variable), nil
	case types.Int, types.UntypedInt:
		return imports.FormatInt(variable), nil
	case types.Int8:
		return imports.FormatInt8(variable), nil
	case types.Int16:
		return imports.FormatInt16(variable), nil
	case types.Int32:
		return imports.FormatInt32(variable), nil
	case types.Int64:
		return imports.FormatInt64(variable), nil
	case types.Uint:
		return imports.FormatUint(variable), nil
	case types.Uint8:
		return imports.FormatUint8(variable), nil
	case types.Uint16:
		return imports.FormatUint16(variable), nil
	case types.Uint32:
		return imports.FormatUint32(variable), nil
	case types.Uint64:
		return imports.FormatUint64(variable), nil
	case types.String:
		return variable, nil
	default:
		return nil, fmt.Errorf("unsupported basic type for path parameters")
	}
}
