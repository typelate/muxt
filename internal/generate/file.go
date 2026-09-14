package generate

import (
	"crypto/sha1"
	"encoding/hex"
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

	"github.com/typelate/muxt/internal/source"
)

// File is one generated Go file: the package it is written into, which the
// types it names are qualified against, and the imports its declarations
// register as they are built.
//
// Every generated file has a File of its own. The imports a file declares
// are then the ones something in it registered, so a file never carries a
// package another file needed.
type File struct {
	pkg                source.Package
	packageIdentifiers map[string]string
	importSpecs        []*ast.ImportSpec
}

func newFile(pkg source.Package) *File {
	return &File{
		pkg:                pkg,
		packageIdentifiers: make(map[string]string),
	}
}

// OutputPackage is the package the generated file is written into.
func (file *File) OutputPackage() source.Package { return file.pkg }

func (file *File) TypeASTExpression(tp types.Type) (ast.Expr, error) {
	s := types.TypeString(tp, file.pkgQualifier)
	return parser.ParseExpr(s)
}

// pkgQualifier implements types.Qualifier
func (file *File) pkgQualifier(pkg *types.Package) string {
	if pkg.Path() == file.pkg.Types.Path() {
		return ""
	}
	return file.Import(pkg.Name(), pkg.Path())
}

func (file *File) Import(pkgIdent, pkgPath string) string {
	if pkgPath == file.pkg.Types.Path() {
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
