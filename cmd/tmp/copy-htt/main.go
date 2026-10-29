// Command copy-htt copies a package directory from the htt project into muxt,
// rewriting import paths and the root package name. Throwaway; deleted later.
//
// Each invocation copies exactly one directory (its .go files, testdata and
// data directories, js_event_attrs.json, and LICENSE), not subdirectories. Packages such as check/internal/*
// need their own invocations.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	httRoot   = "github.com/typelate/htt"
	muxtRoot  = "github.com/typelate/muxt/internal/template"
	internalP = "github.com/typelate/muxt/internal"
)

func rewriteImport(p string) (string, bool) {
	switch {
	case p == httRoot:
		return muxtRoot, true
	case p == httRoot+"/lex":
		return muxtRoot + "/lex", true
	case p == httRoot+"/template":
		return muxtRoot + "/html", true
	case p == httRoot+"/check" || strings.HasPrefix(p, httRoot+"/check/"):
		return internalP + "/check" + strings.TrimPrefix(p, httRoot+"/check"), true
	case p == httRoot+"/analysis":
		return internalP + "/analysis/html", true
	case p == httRoot+"/selectors":
		return muxtRoot + "/selectors", true
	case p == httRoot+"/internal/csstree" || strings.HasPrefix(p, httRoot+"/internal/csstree/"):
		return internalP + "/css/csstree" + strings.TrimPrefix(p, httRoot+"/internal/csstree"), true
	}
	return p, false
}

func main() {
	src := flag.String("src", "", "htt checkout")
	pkg := flag.String("pkg", "", "package directory relative to -src (\".\" for root)")
	dst := flag.String("dst", "", "destination directory")
	flag.Parse()
	if *src == "" || *pkg == "" || *dst == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*src, *pkg, *dst); err != nil {
		log.Fatal(err)
	}
}

func run(src, pkg, dstDir string) error {
	srcDir := filepath.Join(src, pkg)
	isFork := filepath.Clean(pkg) == "template"
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		from := filepath.Join(srcDir, e.Name())
		to := filepath.Join(dstDir, e.Name())
		switch {
		case e.IsDir():
			if copiedDir(e.Name()) {
				if err := copyTree(from, to); err != nil {
					return err
				}
			}
		case strings.HasSuffix(e.Name(), ".go"):
			if err := rewriteGo(from, to, isFork); err != nil {
				return fmt.Errorf("%s: %w", from, err)
			}
		case copiedFile(e.Name()):
			if err := copyFile(from, to); err != nil {
				return err
			}
		}
	}
	return nil
}

// copiedDir reports whether a subdirectory is copied wholesale (testdata and
// embedded data such as csstree's data/mdn).
func copiedDir(name string) bool { return name == "testdata" || name == "data" }

// copiedFile reports whether a non-Go file is copied.
func copiedFile(name string) bool { return name == "js_event_attrs.json" || name == "LICENSE" }

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		return copyFile(p, filepath.Join(to, rel))
	})
}

func rewriteGo(from, to string, isFork bool) error {
	src, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	out, err := transform(from, src, isFork)
	if err != nil {
		return err
	}
	return os.WriteFile(to, out, 0o644)
}

// renameQualifier renames unresolved selector qualifiers from old to new.
func renameQualifier(f *ast.File, old, new string) {
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == old && id.Obj == nil {
				id.Name = new
			}
		}
		return true
	})
}

// transform rewrites one htt source file. isFork is true for files from
// htt/template, whose package becomes html.
func transform(name string, src []byte, isFork bool) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	rootLocal, forkLocal := "", ""
	keepRootName := importsTemplateName(f)
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		np, ok := rewriteImport(p)
		if !ok {
			continue
		}
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
			if alias == "_" || alias == "." {
				return nil, fmt.Errorf("%s: unsupported %q import of %s", name, alias, p)
			}
		}
		switch p {
		case httRoot:
			rootLocal = cmpOr(alias, "htt")
			if keepRootName {
				// Another import already claims "template"; keep the
				// root's local name so uses need no renaming.
				imp.Name = ast.NewIdent(rootLocal)
				rootLocal = ""
			} else {
				imp.Name = nil // the new package name is template
			}
		case httRoot + "/template":
			if alias == "" {
				forkLocal = "template"
			}
		}
		imp.Path.Value = strconv.Quote(np)
	}
	switch f.Name.Name {
	case "htt":
		f.Name.Name = "template"
	case "htt_test":
		f.Name.Name = "template_test"
	}
	if isFork {
		f.Name.Name = strings.Replace(f.Name.Name, "template", "html", 1)
	}
	// Rename the fork's qualifier first: the root's new qualifier is template.
	if forkLocal != "" {
		renameQualifier(f, forkLocal, "html")
	}
	if rootLocal != "" {
		renameQualifier(f, rootLocal, "template")
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// importsTemplateName reports whether a non-htt import is bound to the local
// name "template" (for example text/template).
func importsTemplateName(f *ast.File) bool {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p == httRoot || strings.HasPrefix(p, httRoot+"/") {
			continue
		}
		if imp.Name != nil && imp.Name.Name == "template" || imp.Name == nil && (p == "text/template" || p == "html/template") {
			return true
		}
	}
	return false
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
