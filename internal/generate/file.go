package generate

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
)

type File struct {
	fileSet            *token.FileSet
	typesCache         map[string]*types.Package
	files              map[string]*ast.File
	packages           []*packages.Package
	outPkg             *packages.Package
	packageIdentifiers map[string]string
	importSpecs        []*ast.ImportSpec
}

func newFile(filePath string, fileSet *token.FileSet, list []*packages.Package) (*File, error) {
	if fileSet == nil {
		fileSet = token.NewFileSet()
	}
	file := &File{
		fileSet:            fileSet,
		typesCache:         make(map[string]*types.Package),
		files:              make(map[string]*ast.File),
		packages:           make([]*packages.Package, 0),
		packageIdentifiers: make(map[string]string),
	}
	file.addPackages(list)
	pkg, found := asteval.PackageAtFilepath(list, filePath)
	if !found {
		return nil, fmt.Errorf("package not found for filepath %s", filePath)
	}
	file.outPkg = pkg
	return file, nil
}

func (file *File) Package(path string) (*packages.Package, bool) {
	return asteval.PackageWithPath(file.packages, path)
}

func (file *File) addPackages(packages []*packages.Package) {
	file.packages = slices.Grow(file.packages, len(packages))
	for _, pkg := range packages {
		if pkg == nil {
			continue
		}
		file.typesCache[pkg.PkgPath] = pkg.Types
		file.packages = append(file.packages, pkg)
	}
}

func (file *File) OutputPackage() *packages.Package { return file.outPkg }

func (file *File) SyntaxFile(pos token.Pos) (*ast.File, *token.FileSet, error) {
	position := file.fileSet.Position(pos)
	fSet := token.NewFileSet()
	f, err := parser.ParseFile(fSet, position.Filename, nil, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
	return f, fSet, err
}

func (file *File) TypeASTExpression(tp types.Type) (ast.Expr, error) {
	s := types.TypeString(tp, file.pkgQualifier)
	return parser.ParseExpr(s)
}

// pkgQualifier implements types.Qualifier
func (file *File) pkgQualifier(pkg *types.Package) string {
	if pkg.Path() == file.outPkg.PkgPath {
		return ""
	}
	return file.Import(pkg.Name(), pkg.Path())
}

func (file *File) StructField(pos token.Pos) (*ast.Field, error) {
	f, fileSet, err := file.SyntaxFile(pos)
	if err != nil {
		return nil, err
	}
	position := file.fileSet.Position(pos)
	for _, d := range f.Decls {
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

func (file *File) Types(pkgPath string) (*types.Package, bool) {
	if p, ok := file.typesCache[pkgPath]; ok {
		return p, true
	}
	for _, pkg := range file.packages {
		if pkg.Types.Path() == pkgPath {
			p := pkg.Types
			file.typesCache[pkgPath] = p
			return p, true
		}
	}
	for _, pkg := range file.packages {
		if p, ok := recursivelySearchImports(pkg.Types, pkgPath); ok {
			file.typesCache[pkgPath] = p
			return p, true
		}
	}
	return nil, false
}

func (file *File) Import(pkgIdent, pkgPath string) string {
	if pkgPath == file.outPkg.PkgPath {
		log.Fatal("package path cannot be the same as the output package")
		return ""
	}
	return packageImportName(&file.importSpecs, file.packageIdentifiers, pkgPath, pkgIdent)
}

func (file *File) ImportSpecs() []*ast.ImportSpec {
	result := append(make([]*ast.ImportSpec, 0, len(file.importSpecs)), file.importSpecs...)
	slices.SortFunc(result, func(a, b *ast.ImportSpec) int { return strings.Compare(a.Path.Value, b.Path.Value) })
	return slices.CompactFunc(result, func(a, b *ast.ImportSpec) bool { return a.Path.Value == b.Path.Value })
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

func packageImportName(importSpecs *[]*ast.ImportSpec, packageIdentifiers map[string]string, pkgPath, pkgIdent string) string {
	if ident, ok := packageIdentifiers[pkgPath]; ok {
		return ident
	}
	if pkgIdent == "" {
		pkgIdent = path.Base(pkgPath)
	}
	for existing := range maps.Values(packageIdentifiers) {
		if existing == pkgIdent {
			sum := sha1.New()
			sum.Write([]byte(pkgPath))
			pkgIdent = strings.Join([]string{pkgIdent, hex.EncodeToString(sum.Sum(nil))[:12]}, "")
			break
		}
	}
	var pi *ast.Ident
	if pkgIdent != path.Base(pkgPath) {
		pi = ast.NewIdent(pkgIdent)
	}
	*importSpecs = append(*importSpecs, &ast.ImportSpec{
		Path: &ast.BasicLit{Value: strconv.Quote(pkgPath), Kind: token.STRING},
		Name: pi,
	})
	slices.SortFunc(*importSpecs, func(a, b *ast.ImportSpec) int {
		return strings.Compare(a.Path.Value, b.Path.Value)
	})
	n := pkgIdent
	packageIdentifiers[pkgPath] = n
	return n
}
