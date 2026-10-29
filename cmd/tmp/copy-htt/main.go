// Command copy-htt copies a package directory from the htt project into muxt,
// rewriting import paths and the root package name. Throwaway; deleted later.
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
	if err := run(filepath.Join(*src, *pkg), *dst); err != nil {
		log.Fatal(err)
	}
}

func run(srcDir, dstDir string) error {
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
			if e.Name() == "testdata" {
				if err := copyTree(from, to); err != nil {
					return err
				}
			}
		case strings.HasSuffix(e.Name(), ".go"):
			if err := rewriteGo(from, to); err != nil {
				return fmt.Errorf("%s: %w", from, err)
			}
		case e.Name() == "js_event_attrs.json":
			if err := copyFile(from, to); err != nil {
				return err
			}
		}
	}
	return nil
}

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

func rewriteGo(from, to string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, from, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	rootLocal := ""
	htmlLocal := ""
	for _, imp := range f.Imports {
		if imp.Path.Value == strconv.Quote(httRoot) {
			rootLocal = "htt"
		}
	}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return err
		}
		np, ok := rewriteImport(p)
		if !ok {
			continue
		}
		if p == httRoot {
			if imp.Name != nil {
				rootLocal = imp.Name.Name
			} else {
				rootLocal = "htt"
				imp.Name = nil
			}
		}
		if p == httRoot+"/template" && imp.Name == nil {
			// the fork's package is now named html (the root is named template).
			htmlLocal = "template"
		}
		imp.Path.Value = strconv.Quote(np)
	}
	switch f.Name.Name {
	case "htt":
		f.Name.Name = "template"
	case "htt_test":
		f.Name.Name = "template_test"
	}
	if strings.HasSuffix(filepath.Dir(from), "/htt/template") {
		// htt/template becomes package html.
		f.Name.Name = strings.Replace(f.Name.Name, "template", "html", 1)
	}
	if htmlLocal != "" {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == htmlLocal && id.Obj == nil {
				id.Name = "html"
			}
			return true
		})
	}
	if rootLocal == "htt" {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "htt" && id.Obj == nil {
				id.Name = "template"
			}
			return true
		})
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return err
	}
	return os.WriteFile(to, buf.Bytes(), 0o644)
}
