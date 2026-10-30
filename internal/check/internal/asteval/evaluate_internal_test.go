package asteval

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/template"
)

// createTestDir extracts a txtar fixture into a scratch directory.
func createTestDir(t *testing.T, filename string) string {
	t.Helper()
	dir := t.TempDir()
	archive, err := txtar.ParseFile(filepath.FromSlash(filename))
	require.NoError(t, err)
	tfs, err := txtar.FS(archive)
	require.NoError(t, err)
	require.NoError(t, os.CopyFS(dir, tfs))
	return dir
}

// evalTemplates parses and type-checks the fixture directory's Go files —
// tolerating type errors, since the fixtures deliberately mix invalid
// constructions — then evaluates the named template variable the way the
// package resolves a template construction chain.
func evalTemplates(t *testing.T, dir, varName string, embedFiles ...string) (*template.Template[*types.Signature], error) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	fileSet := token.NewFileSet()
	var files []*ast.File
	for _, match := range matches {
		file, err := parser.ParseFile(fileSet, match, nil, parser.ParseComments|parser.SkipObjectResolution)
		require.NoError(t, err)
		files = append(files, file)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Implicits:  make(map[ast.Node]types.Object),
		Scopes:     make(map[ast.Node]*types.Scope),
		Instances:  make(map[*ast.Ident]types.Instance),
	}
	conf := types.Config{
		Error:    func(error) {},
		Importer: importer.ForCompiler(fileSet, "source", nil),
	}
	pkg, _ := conf.Check("scratch", fileSet, files, info) // fixtures may not fully type check

	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range vs.Names {
					if ident.Name != varName || i >= len(vs.Values) {
						continue
					}
					meta := new(TemplateMetadata)
					ts, _, _, err := EvaluateTemplateSelector(nil, pkg, info, vs.Values[i], dir, varName, "", "", fileSet, files, embedFiles, DefaultFunctions(pkg), make(map[string]any), meta)
					return ts, err
				}
			}
		}
	}
	return nil, fmt.Errorf("variable %s not found", varName)
}

// templateNames lists the names of every template in the evaluated set,
// sorted.
func templateNames(t *testing.T, ts *template.Template[*types.Signature]) []string {
	t.Helper()
	require.NotNil(t, ts)
	names := ts.Names()
	slices.Sort(names)
	return names
}

func lookup(t *testing.T, ts *template.Template[*types.Signature], name string) *template.Template[*types.Signature] {
	t.Helper()
	require.NotNil(t, ts)
	return ts.Lookup(name)
}

func TestEvaluateTemplateSelector(t *testing.T) {
	t.Run("non call", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templatesIdent")
		require.ErrorContains(t, err, "template.go:32:19: expected call expression")
	})
	t.Run("call ParseFS", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/template_ParseFS.txtar"))
		ts, err := evalTemplates(t, dir, "templates", "index.gohtml", "form.gohtml")
		require.NoError(t, err)
		assert.Equal(t, []string{"create", "form.gohtml", "home", "index.gohtml", "update"}, templateNames(t, ts))
	})
	t.Run("call ParseFS with assets dir", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/assets_dir.txtar"))
		ts, err := evalTemplates(t, dir, "templates", "assets/index.gohtml", "assets/form.gohtml")
		require.NoError(t, err)
		assert.Equal(t, []string{"create", "form.gohtml", "home", "index.gohtml", "update"}, templateNames(t, ts))
	})
	t.Run("call New", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		ts, err := evalTemplates(t, dir, "templateNew", "index.gohtml")
		require.NoError(t, err)
		assert.Equal(t, []string{"some-name"}, templateNames(t, ts))
	})
	t.Run("call New after calling ParseFS", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		ts, err := evalTemplates(t, dir, "templateParseFSNew", "index.gohtml")
		require.NoError(t, err)
		assert.Equal(t, []string{"greetings", "index.gohtml"}, templateNames(t, ts))
	})
	t.Run("call New before calling ParseFS", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		ts, err := evalTemplates(t, dir, "templateNewParseFS", "index.gohtml")
		require.NoError(t, err)
		assert.Equal(t, []string{"greetings", "index.gohtml"}, templateNames(t, ts))
	})
	t.Run("call New with no args", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateNewMissingArg", "index.gohtml")
		require.ErrorContains(t, err, "expected exactly one string literal argument")
	})
	t.Run("call New on unknown X", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateWrongX", "index.gohtml")
		require.ErrorContains(t, err, "template.go:20:19: expected template")
	})
	t.Run("call New with wrong arg count", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateWrongArgCount", "index.gohtml")
		require.ErrorContains(t, err, "template.go:22:38: expected exactly one string literal argument")
	})
	t.Run("call New on unexpected X", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateNewOnIndexed", "index.gohtml")
		require.ErrorContains(t, err, "template.go:24:25: expected exactly one argument ts[0] got 2")
	})
	t.Run("call New with non string literal arg", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateNewArg42", "index.gohtml")
		require.ErrorContains(t, err, "template.go:26:34: expected string literal got 42")
	})
	t.Run("call New with non literal arg", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateNewArgIdent", "index.gohtml")
		require.ErrorContains(t, err, "template.go:28:37: expected string literal got TemplateName")
	})
	t.Run("call New with upstream error", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateNewErrUpstream", "index.gohtml")
		require.ErrorContains(t, err, "template.go:30:40: expected string literal got fail")
	})
	t.Run("unsupported function", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "unsupportedMethod", "index.gohtml")
		require.ErrorContains(t, err, "template.go:34:22: unsupported function Unknown")
	})
	t.Run("call Must with unexpected function expression", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "unexpectedFunExpression", "index.gohtml")
		require.ErrorContains(t, err, "template.go:36:28: unexpected expression *ast.IndexExpr: x[3]")
	})
	t.Run("call Must on non ident receiver", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateMustNonIdentReceiver", "index.gohtml")
		require.ErrorContains(t, err, "template.go:38:33: unexpected expression *ast.Ident: f")
	})
	t.Run("call Must with two arguments", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateMustCalledWithTwoArgs", "index.gohtml")
		require.ErrorContains(t, err, "template.go:40:47: expected exactly one argument template got 2")
	})
	t.Run("call Must with no argument", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateMustCalledWithNoArg", "index.gohtml")
		require.ErrorContains(t, err, "template.go:42:47: expected exactly one argument template got 0")
	})
	t.Run("call Must wrong template package ident", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateMustWrongPackageIdent", "index.gohtml")
		require.ErrorContains(t, err, "template.go:44:34: expected template")
	})
	t.Run("call ParseFS wrong template package ident", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateParseFSWrongPackageIdent", "index.gohtml")
		require.ErrorContains(t, err, "template.go:46:37: expected template")
	})
	t.Run("call ParseFS receiver errored", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateParseFSReceiverErr", "index.gohtml")
		require.ErrorContains(t, err, "template.go:48:43: expected exactly one string literal argument")
	})
	t.Run("call ParseFS unexpected receiver", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateParseFSUnexpectedReceiver", "index.gohtml")
		require.ErrorContains(t, err, "template.go:50:38: expected exactly one argument x[0] got 2")
	})
	t.Run("call ParseFS with no arguments", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateParseFSNoArgs", "index.gohtml")
		require.ErrorContains(t, err, "template.go:52:42: missing required arguments")
	})
	t.Run("call ParseFS with first arg non ident", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateParseFSFirstArgNonIdent", "index.gohtml")
		require.ErrorContains(t, err, "template.go:54:53: first argument to ParseFS must be an identifier")
	})
	t.Run("call ParseFS with non string literal glob", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateParseFSNonStringLiteralGlob", "index.gohtml")
		require.ErrorContains(t, err, "template.go:56:78: expected string literal got 42")
	})
	t.Run("call ParseFS with bad glob", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateParseFSWithBadGlob", "index.gohtml")
		require.ErrorContains(t, err, `template.go:58:64: bad pattern "[fail": syntax error in pattern`)
	})
	t.Run("relative template path fails", func(t *testing.T) {
		_, err := RelativeFilePaths(t.TempDir(), "\x00/index.gohtml") // null must not be in a path
		require.ErrorContains(t, err, "Rel: can't make")
	})
	t.Run("call ParseFS and filter filepaths by globs", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/template_ParseFS.txtar"))
		tsHTML, err := evalTemplates(t, dir, "templatesHTML", "index.gohtml", "script.html")
		require.NoError(t, err)
		tsGoHTML, err := evalTemplates(t, dir, "templatesGoHTML", "index.gohtml", "script.html")
		require.NoError(t, err)
		assert.NotNil(t, lookup(t, tsHTML, "script.html"))
		assert.NotNil(t, lookup(t, tsHTML, "console_log"))
		assert.Nil(t, lookup(t, tsGoHTML, "script.html"))
		assert.Nil(t, lookup(t, tsGoHTML, "console_log"))
	})
	t.Run("call bad embed pattern", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/bad_embed_pattern.txtar"))
		_, err := evalTemplates(t, dir, "templates", "greeting.gohtml")
		require.ErrorContains(t, err, `template.go:9:2: embed comment malformed: syntax error in pattern`)
	})
	t.Run("embed variable not found", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/template_ParseFS.txtar"))
		_, err := evalTemplates(t, dir, "templateEmbedVariableNotFound", "index.gohtml")
		require.ErrorContains(t, err, `template.go:22:65: variable hiding not found`)
	})
	t.Run("multiple delimiter types", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/delims.txtar"))
		ts, err := evalTemplates(t, dir, "templates", "default.gohtml", "triple_parens.gohtml", "double_square.gohtml")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"", "default", "default.gohtml", "double_square.gohtml", "parens", "square", "triple_parens.gohtml"}, templateNames(t, ts))
	})
	t.Run("New method call gets no args", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateNewHasWrongNumberOfArgs", "index.gohtml")
		require.ErrorContains(t, err, `template.go:60:101: expected exactly one string literal argument`)
	})
	t.Run("New method call gets wrong type of args", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateNewHasWrongTypeOfArgs", "index.gohtml")
		require.ErrorContains(t, err, `template.go:62:56: expected string literal got 9000`)
	})
	t.Run("New method call gets too many args", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateNewHasTooManyArgs", "index.gohtml")
		require.ErrorContains(t, err, `template.go:64:51: expected exactly one string literal argument`)
	})
	t.Run("Delims method call gets no args", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateDelimsGetsNoArgs", "index.gohtml")
		require.ErrorContains(t, err, `template.go:66:53: expected exactly two string literal arguments`)
	})
	t.Run("Delims method call gets too many args", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateDelimsGetsTooMany", "index.gohtml")
		require.ErrorContains(t, err, `template.go:68:54: expected exactly two string literal arguments`)
	})
	t.Run("Delims have wrong type of argument expressions", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateDelimsWrongExpressionArg", "index.gohtml")
		require.ErrorContains(t, err, `template.go:70:67: expected string literal got y`)
	})
	t.Run("ParseFS method fails", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateParseFSMethodFails", "index.gohtml")
		require.ErrorContains(t, err, `template.go:72:73: expected string literal got fail`)
	})
	t.Run("Options method requires string literals", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateOptionsRequiresStringLiterals", "index.gohtml")
		require.ErrorContains(t, err, `template.go:74:67: expected string literal got fail`)
	})
	t.Run("unknown method", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateUnknownMethod", "index.gohtml")
		require.ErrorContains(t, err, `template.go:76:26: unsupported method Unknown`)
	})
	t.Run("Option call", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateOptionCall", "index.gohtml")
		require.NoError(t, err)
	})
	t.Run("Option call wrong argument", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/templates.txtar"))
		_, err := evalTemplates(t, dir, "templateOptionCallUnknownArg", "index.gohtml")
		require.ErrorContains(t, err, "unknown option")
	})
	t.Run("Funcs call", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templates", "greet.gohtml")
		require.NoError(t, err)
	})
	t.Run("Func not defined", func(t *testing.T) {
		// As html/template's parser does, the parser rejects a call of
		// a function the set does not define.
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesFuncNotDefined", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `missing_func.gohtml:1: function "enemy" not defined`)
	})
	t.Run("Func wrong parameter kind", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesWrongArg", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `expected a template.FuncMap composite literal got wrong`)
	})
	t.Run("Func wrong too many args", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesTwoArgs", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `expected exactly 1 template.FuncMap composite literal argument`)
	})
	t.Run("Func wrong no args", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesNoArgs", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `expected exactly 1 template.FuncMap composite literal argument`)
	})
	t.Run("Func wrong package ident", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesWrongTypePackageName", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `expected a template.FuncMap composite literal got wrong.FuncMap{}`)
	})
	t.Run("Func wrong Type ident", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesWrongTypeName", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `expected a template.FuncMap composite literal got template.Wrong{}`)
	})
	t.Run("Func wrong Type", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesWrongTypeExpression", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `expected a template.FuncMap composite literal got wrong{}`)
	})
	t.Run("Func wrong elem", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesWrongTypeElem", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `expected element at index 0 to be a key value pair got wrong`)
	})
	t.Run("Func wrong elem key", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/funcs.txtar"))
		_, err := evalTemplates(t, dir, "templatesWrongElemKey", "missing_func.gohtml", "greet.gohtml")
		require.ErrorContains(t, err, `expected string literal got wrong`)
	})
	t.Run("Parse template name from new", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/parse.txtar"))
		ts, err := evalTemplates(t, dir, "templates")
		require.NoError(t, err)
		assert.NotNil(t, lookup(t, ts, "GET /"))
	})
	t.Run("Parse string has multiple routes", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/parse.txtar"))
		ts, err := evalTemplates(t, dir, "multiple")
		require.NoError(t, err)
		assert.NotNil(t, lookup(t, ts, "GET /"))
		assert.NotNil(t, lookup(t, ts, "GET /{name}"))
	})
	t.Run("Parse is missing argument", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/parse.txtar"))
		_, err := evalTemplates(t, dir, "noArg")
		require.ErrorContains(t, err, "parse.go:12:35: expected exactly one string literal argument")
	})
	t.Run("Parse gets wrong argument type", func(t *testing.T) {
		dir := createTestDir(t, filepath.FromSlash("testdata/template/parse.txtar"))
		_, err := evalTemplates(t, dir, "wrongArg")
		require.ErrorContains(t, err, "parse.go:14:40: expected string literal got 500")
	})
}

func TestParseTemplateNames(t *testing.T) {
	for _, tt := range []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "quoted globs with double quotes",
			input:    `*.txt "*.md" "images/*.png"`,
			expected: []string{"*.txt", "*.md", "images/*.png"},
		},
		{
			name:     "quoted globs with backticks",
			input:    "*.go `*.js` `css/*.css`",
			expected: []string{"*.go", "*.js", "css/*.css"},
		},
		{
			name:     "glob with spaces",
			input:    `"file with spaces.txt"`,
			expected: []string{"file with spaces.txt"},
		},
		{
			name:     "unclosed quote",
			input:    `"unclosed quote`,
			expected: []string{"unclosed quote"},
		},
		{
			name:     "plain files",
			input:    "plain `other`",
			expected: []string{"plain", "other"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseTemplateNames(tt.input))
		})
	}
}
