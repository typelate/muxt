package generate

import (
	"crypto/sha1"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/muxt"
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
	// taken are the names an import may not have: a name the output
	// package declares, and one the generated code declares where it
	// spells an imported package's types.
	taken map[string]bool
	// reserved are the names newFile was passed, for the other files of
	// the run.
	reserved []string
}

// newFile returns a File for a generated file in pkg. reserved are more
// names its imports may not have, such as the names of the declarations
// the run generates.
func newFile(pkg source.Package, reserved ...string) *File {
	taken := make(map[string]bool)
	for _, names := range [][]string{generatedLocalIdentifiers, types.Universe.Names(), reserved} {
		for _, name := range names {
			taken[name] = true
		}
	}
	if pkg.Types != nil {
		for _, name := range pkg.Types.Scope().Names() {
			taken[name] = true
		}
	}
	return &File{
		pkg:                pkg,
		packageIdentifiers: make(map[string]string),
		taken:              taken,
		reserved:           reserved,
	}
}

// sibling returns a File for another file of the same run: the same
// package and reserved names, and no imports.
func (file *File) sibling() *File { return newFile(file.pkg, file.reserved...) }

// Identifiers generated code declares in a function where it may spell a
// type from an imported package.
const (
	bufIdent             = "buf"
	templateDataVarIdent = "td"
	statusCodeIdent      = "statusCode"
	defaultStatusIdent   = "defaultStatusCode"
	flusherIdent         = "flusher"
	okIdent              = "ok"
	mutexIdent           = "mut"
	headerIdent          = "h"
	resultIdent          = "result"
	payloadIdent         = "payload"
	executedIdent        = "executed"
	executeDataIdent     = "data"
	bodyValueIdent       = "bodyValue"
	jsonBodyIdent        = "jsonBody"
	formValueIdent       = "val"
	formFieldValueIdent  = "value"
	fileHeadersIdent     = "fhs"
	contentTypeIdent     = "contentType"
)

// generatedLocalIdentifiers are the names in scope in the generated
// routes function, its handlers and their closures. An import with one of
// these names would be shadowed where the handler spells its types, so it
// is given an alias instead. Locals named for a template's path
// parameters end in PathParam or Parsed, and the results of nested calls
// are result0, result1 and so on; packages with names like those are not
// expected.
//
// The multipart argument is left out: it is mime/multipart's name, and the
// handler spells that package's types only before declaring the local
// (var multipart *multipart.Form = request.MultipartForm).
var generatedLocalIdentifiers = []string{
	muxParamName,
	receiverIdent,
	loggerIdent,
	pathPrefixPathsStructFieldName,
	middlewareParamName,
	bufferPoolIdent,
	errIdent,
	resultStatusCodeIdent,
	bufIdent,
	templateDataVarIdent,
	statusCodeIdent,
	defaultStatusIdent,
	flusherIdent,
	okIdent,
	mutexIdent,
	headerIdent,
	resultIdent,
	payloadIdent,
	executedIdent,
	executeDataIdent,
	bodyValueIdent,
	jsonBodyIdent,
	formValueIdent,
	formFieldValueIdent,
	fileHeadersIdent,
	contentTypeIdent,
	muxt.TemplateNameScopeIdentifierHTTPRequest,
	muxt.TemplateNameScopeIdentifierHTTPResponse,
	muxt.TemplateNameScopeIdentifierContext,
	muxt.TemplateNameScopeIdentifierForm,
	muxt.TemplateNameScopeIdentifierRequestBody,
	muxt.TemplateNameScopeIdentifierLastEventID,
	muxt.TemplateNameScopeIdentifierSignals,
}

// TypeExpr spells t as the generated file refers to it, importing what it
// needs.
func (file *File) TypeExpr(t source.Type) (ast.Expr, error) {
	return parser.ParseExpr(t.Format(file.qualify))
}

func (file *File) qualify(pkgName, pkgPath string) string {
	if pkgPath == file.pkg.Types.Path() {
		return ""
	}
	return file.Import(pkgName, pkgPath)
}

func (file *File) Import(pkgIdent, pkgPath string) string {
	if pkgPath == file.pkg.Types.Path() {
		// qualify spells the output package's own names unqualified, and
		// the generators import only other packages, so this is a bug in
		// a generator.
		panic("generate: a generated file cannot import its own package " + pkgPath)
	}
	return packageImportName(&file.importSpecs, file.packageIdentifiers, file.taken, pkgPath, pkgIdent)
}

func (file *File) ImportSpecs() []*ast.ImportSpec {
	result := slices.Clone(file.importSpecs)
	slices.SortFunc(result, func(a, b *ast.ImportSpec) int { return strings.Compare(a.Path.Value, b.Path.Value) })
	return slices.CompactFunc(result, func(a, b *ast.ImportSpec) bool { return a.Path.Value == b.Path.Value })
}

// packageImportName registers pkgPath and returns the name the file refers
// to it by: pkgIdent, or the path's last element, followed by a hash of
// the path when another import or a taken name has it.
func packageImportName(importSpecs *[]*ast.ImportSpec, packageIdentifiers map[string]string, taken map[string]bool, pkgPath, pkgIdent string) string {
	if ident, ok := packageIdentifiers[pkgPath]; ok {
		return ident
	}
	if pkgIdent == "" {
		pkgIdent = path.Base(pkgPath)
	}
	if taken[pkgIdent] || slices.Contains(slices.Collect(maps.Values(packageIdentifiers)), pkgIdent) {
		sum := sha1.New()
		sum.Write([]byte(pkgPath))
		pkgIdent = strings.Join([]string{pkgIdent, hex.EncodeToString(sum.Sum(nil))[:12]}, "")
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
