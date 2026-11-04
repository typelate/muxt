// Command gen copies muxt's template engine into internal/overlay/_engine so
// it can be embedded and laid over $GOROOT/src/html/template.
//
// The copy is the html/template-shaped facade (internal/template/html) and
// its dependency closure (internal/template, internal/template/lex,
// golang.org/x/net/html and golang.org/x/net/html/atom), with import paths
// rewritten under html/template/internal, the engine package renamed so it
// does not clash with the facade's package template, and the facade's
// aliases to the standard library's html/template replaced by definitions
// (the facade is html/template once overlaid). Test files are not copied.
//
// Run it with go generate ./internal/overlay.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/printer"
	"go/token"
	htmltemplate "html/template"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

func main() {
	out := flag.String("o", "_engine", "directory to write the engine copy into; it is replaced")
	flag.Parse()
	if err := run(context.Background(), *out); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, out string) error {
	sources, err := listSources(ctx)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	return generate(sources, out)
}

// copies names each package in the copy, where it goes under
// $GOROOT/src/html/template, and the package clause it gets ("" keeps it).
var copies = []struct {
	importPath, dest, clause string
}{
	{"github.com/typelate/muxt/internal/template/html", ".", "template"},
	{"github.com/typelate/muxt/internal/template", "internal/engine", "engine"},
	{"github.com/typelate/muxt/internal/template/lex", "internal/lex", ""},
	{"golang.org/x/net/html", "internal/xhtml", ""},
	{"golang.org/x/net/html/atom", "internal/xhtml/atom", ""},
}

// overlaidPath is the import path the facade has once it is overlaid.
const overlaidPath = "html/template"

type source struct {
	ImportPath string
	Name       string
	Dir        string
	EmbedFiles []string
	Module     struct{ Dir string }
}

// listSources asks the go command where the copied packages are.
func listSources(ctx context.Context) ([]source, error) {
	args := []string{"list", "-json=ImportPath,Name,Dir,EmbedFiles,Module"}
	for _, c := range copies {
		args = append(args, c.importPath)
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, stderr.Bytes())
	}
	var sources []source
	dec := json.NewDecoder(bytes.NewReader(stdout))
	for {
		var s source
		if err := dec.Decode(&s); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decoding go list output: %w", err)
		}
		sources = append(sources, s)
	}
	if len(sources) != len(copies) {
		return nil, fmt.Errorf("go list returned %d packages, want %d", len(sources), len(copies))
	}
	return sources, nil
}

// generate writes the copy of sources into out.
func generate(sources []source, out string) error {
	byPath := make(map[string]source, len(sources))
	for _, s := range sources {
		byPath[s.ImportPath] = s
	}
	r := rules{paths: make(map[string]string), renames: make(map[string]rename)}
	for _, c := range copies {
		s, ok := byPath[c.importPath]
		if !ok {
			return fmt.Errorf("no source for %s", c.importPath)
		}
		to := path.Join(overlaidPath, c.dest)
		r.paths[c.importPath] = to
		if c.clause != "" && c.clause != s.Name {
			r.renames[to] = rename{from: s.Name, to: c.clause}
		}
	}
	for _, c := range copies {
		dest := filepath.Join(out, filepath.FromSlash(c.dest))
		if err := r.copyPackage(byPath[c.importPath], dest, c.clause); err != nil {
			return fmt.Errorf("copying %s: %w", c.importPath, err)
		}
	}
	return nil
}

func (r rules) copyPackage(s source, dest, clause string) error {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(s.Dir, name))
		if err != nil {
			return err
		}
		out, keep, err := r.rewriteFile(name, src, clause)
		if err != nil {
			return err
		}
		if !keep {
			continue
		}
		if err := os.WriteFile(filepath.Join(dest, name), out, 0o644); err != nil {
			return err
		}
	}
	for _, name := range s.EmbedFiles {
		b, err := os.ReadFile(filepath.Join(s.Dir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		to := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(to, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// rules says how to rewrite the copied files.
type rules struct {
	// paths maps an import path in the live source to its path in the copy.
	paths map[string]string
	// renames holds the copied packages whose package clause changes, by
	// their path in the copy, so references to them are renamed too.
	renames map[string]rename
}

type rename struct{ from, to string }

// stdlibContent holds the html/template types the facade aliases. Under the
// overlay the facade is html/template, so each alias becomes a definition
// with the same underlying type.
var stdlibContent = map[string]reflect.Type{
	"CSS":       reflect.TypeFor[htmltemplate.CSS](),
	"HTML":      reflect.TypeFor[htmltemplate.HTML](),
	"HTMLAttr":  reflect.TypeFor[htmltemplate.HTMLAttr](),
	"JS":        reflect.TypeFor[htmltemplate.JS](),
	"JSStr":     reflect.TypeFor[htmltemplate.JSStr](),
	"Srcset":    reflect.TypeFor[htmltemplate.Srcset](),
	"URL":       reflect.TypeFor[htmltemplate.URL](),
	"ErrorCode": reflect.TypeFor[htmltemplate.ErrorCode](),
}

// rewriteFile returns the copy of one Go file, or keep false when the file
// is excluded from every build (//go:build ignore). A non-empty clause
// replaces the package clause.
func (r rules) rewriteFile(name string, src []byte, clause string) (_ []byte, keep bool, _ error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}
	ignored, err := ignoredByConstraint(f)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", name, err)
	}
	if ignored {
		return nil, false, nil
	}
	if clause != "" {
		f.Name.Name = clause
	}
	if err := defineStdlibAliases(fset, f); err != nil {
		return nil, false, fmt.Errorf("%s: %w", name, err)
	}
	for _, spec := range f.Imports {
		old, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, false, err
		}
		to, ok := r.paths[old]
		if !ok {
			continue
		}
		spec.Path.Value = strconv.Quote(to)
		if rn, ok := r.renames[to]; ok && spec.Name == nil {
			renameReferences(f, rn)
		}
	}
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&buf, fset, f); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

// ignoredByConstraint reports whether the file's //go:build line is the
// ignore tag, which excludes it from every build.
func ignoredByConstraint(f *ast.File) (bool, error) {
	for _, group := range f.Comments {
		if group.Pos() >= f.Package {
			break
		}
		for _, c := range group.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}
			expr, err := constraint.Parse(c.Text)
			if err != nil {
				return false, err
			}
			if tag, ok := expr.(*constraint.TagExpr); ok && tag.Tag == "ignore" {
				return true, nil
			}
		}
	}
	return false, nil
}

// renameReferences rewrites pkg.X to the package's new name, where pkg is
// an unresolved identifier, so it names the import and not a local.
func renameReferences(f *ast.File, rn rename) {
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == rn.from && id.Obj == nil {
			id.Name = rn.to
		}
		return true
	})
}

// defineStdlibAliases replaces type X = template.X, where template is the
// standard library's html/template, with a definition of X.
func defineStdlibAliases(fset *token.FileSet, f *ast.File) error {
	name, ok := importName(f, "html/template")
	if !ok {
		return nil
	}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts := spec.(*ast.TypeSpec)
			sel, ok := ts.Type.(*ast.SelectorExpr)
			if !ts.Assign.IsValid() || !ok {
				continue
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != name || id.Obj != nil {
				continue
			}
			typ, ok := stdlibContent[sel.Sel.Name]
			if !ok {
				return fmt.Errorf("type %s aliases html/template.%s, which has no known definition", ts.Name.Name, sel.Sel.Name)
			}
			ts.Assign = token.NoPos
			ts.Type = &ast.Ident{Name: typ.Kind().String(), NamePos: sel.Pos()}
		}
	}
	if !astutil.UsesImport(f, "html/template") {
		astutil.DeleteImport(fset, f, "html/template")
	}
	return nil
}

func importName(f *ast.File, importPath string) (string, bool) {
	for _, spec := range f.Imports {
		if p, err := strconv.Unquote(spec.Path.Value); err != nil || p != importPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, true
		}
		return path.Base(importPath), true
	}
	return "", false
}
