package muxt

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template/parse"
)

//go:embed nodes.go
var nodesGoSource string

func Generate(directory, fileName string, line int, args []string) error {
	var (
		generatedFileSuffix string
	)

	var flags flag.FlagSet
	flags.StringVar(&generatedFileSuffix, "suffix", "_gen.go", "a suffix to add to the name of generated files")
	if err := flags.Parse(args); err != nil {
		return err
	}
	p := filepath.Join(directory, fileName)
	buf, err := os.ReadFile(p)
	if err != nil {
		return err
	}

	tokenSet := token.NewFileSet()
	file, err := parser.ParseFile(tokenSet, fileName, buf, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return err
	}

	for _, d := range file.Decls {
		switch node := d.(type) {
		case *ast.GenDecl:
			if node.Tok != token.VAR {
				continue
			}
			for _, spec := range node.Specs {
				v, ok := spec.(*ast.ValueSpec)
				if !ok || v.Doc == nil {
					continue
				}
				commentPosition := tokenSet.Position(v.Doc.Pos())
				if commentPosition.Line != line {
					continue
				}
				for j := 0; j < len(v.Names) && j < len(v.Values) && len(v.Names) == len(v.Values); j++ {
					if err := generateHandlersFunction(directory, generatedFileSuffix, tokenSet, file, v.Names[j], v.Values[j]); err != nil {
						return err
					}
				}
				break
			}
		}
	}

	info, err := os.Stat(p)
	if err != nil {
		return err
	}

	var out bytes.Buffer
	if err := format.Node(&out, tokenSet, file); err != nil {
		return err
	}

	return os.WriteFile(p, out.Bytes(), info.Mode())
}

func clearPositionInformation(n ast.Node) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.File:
			node.Comments = nil
		case *ast.Comment:
			node.Slash = token.NoPos
		case *ast.GenDecl:
			node.Rparen = token.NoPos
			node.Lparen = token.NoPos
		case *ast.Ident:
			node.NamePos = token.NoPos
		case *ast.BasicLit:
			node.ValuePos = token.NoPos
		case *ast.BlockStmt:
			node.Lbrace = token.NoPos
			node.Rbrace = token.NoPos
		case *ast.FieldList:
			node.Opening = token.NoPos
			node.Closing = token.NoPos
		case *ast.Ellipsis:
			node.Ellipsis = token.NoPos
		case *ast.CompositeLit:
			node.Rbrace = token.NoPos
			node.Lbrace = token.NoPos
		case *ast.ParenExpr:
			node.Rparen = token.NoPos
			node.Lparen = token.NoPos
		case *ast.IndexExpr:
			node.Lbrack = token.NoPos
			node.Rbrack = token.NoPos
		case *ast.IndexListExpr:
			node.Lbrack = token.NoPos
			node.Rbrack = token.NoPos
		case *ast.SliceExpr:
			node.Lbrack = token.NoPos
			node.Rbrack = token.NoPos
		case *ast.TypeAssertExpr:
			node.Rparen = token.NoPos
			node.Lparen = token.NoPos
		case *ast.CallExpr:
			node.Rparen = token.NoPos
			node.Lparen = token.NoPos
			node.Ellipsis = token.NoPos
		case *ast.StarExpr:
			node.Star = token.NoPos
		case *ast.UnaryExpr:
			node.OpPos = token.NoPos
		case *ast.BinaryExpr:
			node.OpPos = token.NoPos
		case *ast.KeyValueExpr:
			node.Colon = token.NoPos
		case *ast.ArrayType:
			node.Lbrack = token.NoPos
		case *ast.StructType:
			node.Struct = token.NoPos
		case *ast.FuncType:
			node.Func = token.NoPos
		case *ast.InterfaceType:
			node.Interface = token.NoPos
		case *ast.MapType:
			node.Map = token.NoPos
		case *ast.ChanType:
			node.Begin = token.NoPos
			node.Arrow = token.NoPos
		}
		return true
	})
}

func ensureImport(file *ast.File, pkgPath string) *ast.Ident {
	for _, im := range file.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		if p != pkgPath {
			continue
		}
		if im.Name != nil {
			return im.Name
		}
		return ast.NewIdent(path.Base(p))
	}
	for _, decl := range file.Decls {
		imports, ok := decl.(*ast.GenDecl)
		if !ok || imports.Tok != token.IMPORT {
			continue
		}
		in := &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(pkgPath),
			},
		}
		imports.Specs = append(imports.Specs, in)
		file.Imports = append(file.Imports, in)

	}
	return ast.NewIdent(path.Base(pkgPath))
}

func generateHandlersFunction(directory, generatedSuffix string, tokenSet *token.FileSet, file *ast.File, templatesVariable *ast.Ident, expression ast.Expr) error {
	i := slices.IndexFunc(file.Decls, func(decl ast.Decl) bool {
		fd, ok := decl.(*ast.FuncDecl)
		return ok && fd.Name.Name == fnIdent.Name
	})

	var templatesPackageIdent *ast.Ident
	for _, in := range file.Imports {
		if in.Path.Kind != token.STRING {
			continue
		}
		p, err := strconv.Unquote(in.Path.Value)
		if err != nil || p != "html/template" {
			continue
		}
		if in.Name == nil {
			templatesPackageIdent = ast.NewIdent("template")
		} else {
			templatesPackageIdent = in.Name
		}
		break
	}

	if templatesPackageIdent == nil {
		return fmt.Errorf("failed to determine package name for html/template")
	}
	templateFilenames := make(map[*template.Template]string)
	t, err := templatesMust(directory, tokenSet, templatesPackageIdent, templatesVariable, expression, templateFilenames)
	if err != nil {
		return err
	}

	generatedSuffix = strings.TrimSuffix(generatedSuffix, ".go") + ".go"

	for _, ts := range t.Templates() {
		_, err, ok := NewEndpointDefinition(ts.Name())
		if !ok {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func errNotMust(ident *ast.Ident) error {
	return fmt.Errorf("expected right hand side of %s to be template.Must", ident.Name)
}

func templatesMust(directory string, tokenSet *token.FileSet, templatesPackageIdent, variable *ast.Ident, expression ast.Expr, templateFilenames map[*template.Template]string) (*template.Template, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return nil, errNotMust(variable)
	}
	return templatesFactory(directory, tokenSet, templatesPackageIdent, variable, call, 0)
}

func templatesFactory(directory string, tokenSet *token.FileSet, templatesPackageIdent, variable *ast.Ident, expression ast.Expr, depth int) (*template.Template, error) {
	if call, ok := expression.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Must" {
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("expected %s.%s", templatesPackageIdent.Name, sel.Sel.Name)
			}
			if pkg.Name != templatesPackageIdent.Name {
				return nil, fmt.Errorf("expected %s.%s", templatesPackageIdent.Name, sel.Sel.Name)
			}
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("expected %s.%s", templatesPackageIdent.Name, sel.Sel.Name)
			}
			return templatesFactory(directory, tokenSet, templatesPackageIdent, variable, call.Args[0], depth+1)
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ParseFS" {
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("expected %s.%s", templatesPackageIdent.Name, sel.Sel.Name)
			}
			if pkg.Name != templatesPackageIdent.Name {
				return nil, fmt.Errorf("expected %s.%s", templatesPackageIdent.Name, sel.Sel.Name)
			}
			if len(call.Args) < 1 {
				return nil, fmt.Errorf("%s.%s is missing arguments", templatesPackageIdent.Name, sel.Sel.Name)
			}
			fsIdent, ok := call.Args[0].(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("%s.%s expected a variable with type embed.FS as the first argument", templatesPackageIdent.Name, sel.Sel.Name)
			}
			var filePaths []string
			valSpec, ok := fsIdent.Obj.Decl.(*ast.ValueSpec)
			for _, line := range valSpec.Doc.List {
				comment := strings.TrimSpace(strings.TrimPrefix(line.Text, "//"))
				if !strings.HasPrefix(comment, "go:embed ") {
					continue
				}
				for _, pattern := range strings.Fields(strings.TrimPrefix(comment, "go:embed ")) {
					matches, err := filepath.Glob(filepath.Join(directory, filepath.FromSlash(pattern)))
					if err != nil {
						return nil, fmt.Errorf("failed to match pattern %s from %s", pattern, tokenSet.Position(line.Pos()))
					}
					for _, match := range matches {
						filePaths = append(filePaths, match)
					}
				}
			}

			filtered := filePaths[:0]
			for _, filePath := range filePaths {
				rel, err := filepath.Rel(directory, filePath)
				if err != nil {
					return nil, err
				}
				for _, arg := range call.Args[1:] {
					lit, ok := arg.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return nil, fmt.Errorf("argument at %s must be a string literal", tokenSet.Position(arg.Pos()))
					}
					value, _ := strconv.Unquote(lit.Value)
					pattern := filepath.FromSlash(value)
					if matched, err := filepath.Match(pattern, rel); err == nil && matched {
						filtered = append(filtered, filePath)
					}
				}
			}
			filePaths = filtered

			var root *template.Template
			for _, p := range filePaths {
				r, err := filepath.Rel(directory, p)
				if err != nil {
					return nil, err
				}

				n := filepath.Base(p)
				t := template.New(n)
				t.Tree = parse.New(n)
				t.Tree.ParseName = r

				b, err := os.ReadFile(p)
				if err != nil {
					return nil, err
				}
				t, err = t.Parse(string(b))
				if err != nil {
					return nil, err
				}

				for _, ts := range t.Templates() {
					if root == nil {
						root = t
						break
					}
					root, err = root.AddParseTree(ts.Tree.Name, ts.Tree)
					if err != nil {
						return nil, err
					}
				}
			}
			return root, nil
		}
	}
	return nil, fmt.Errorf("failed to evaluate template expression at %s", tokenSet.Position(expression.Pos()))
}

const (
	// language=go
	executeFunctionLiteral = `func (res http.ResponseWriter, req *http.Request, code int, t *template.Template, data any) {
	b := bytes.NewBuffer(nil)
	if err := t.Execute(b, data); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.Header().Set("content-type", "text/html; charset=utf-8")
	res.Header().Set("content-length", strconv.Itoa(b.Len()))
	res.WriteHeader(code)
	_, _ = b.WriteTo(res)
}`
)
