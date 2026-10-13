package astgen_test

import (
	"encoding/json/v2"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/source"
)

// imports records the packages a builder registers.
type imports map[string]string

func (im imports) Import(ident, pkgPath string) string {
	if ident == "" {
		ident = path.Base(pkgPath)
	}
	im[pkgPath] = ident
	return ident
}

func (im imports) ImportSpecs() []*ast.ImportSpec { return nil }

// assignBlank wraps an expression in a statement, which Format can print on
// its own where a bare function literal is not a declaration.
func assignBlank(expr ast.Expr) ast.Stmt {
	return &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr}}
}

func TestConvertToString(t *testing.T) {
	for _, tt := range []struct {
		name string
		kind types.BasicKind
		want string
	}{
		{name: "bool", kind: types.Bool, want: "strconv.FormatBool(bool(v))"},
		{name: "untyped bool", kind: types.UntypedBool, want: "strconv.FormatBool(bool(v))"},
		{name: "int", kind: types.Int, want: "strconv.Itoa(v)"},
		{name: "untyped int", kind: types.UntypedInt, want: "strconv.Itoa(v)"},
		{name: "int8", kind: types.Int8, want: "strconv.FormatInt(int64(v), 10)"},
		{name: "int16", kind: types.Int16, want: "strconv.FormatInt(int64(v), 10)"},
		{name: "int32", kind: types.Int32, want: "strconv.FormatInt(int64(v), 10)"},
		{name: "int64", kind: types.Int64, want: "strconv.FormatInt(int64(v), 10)"},
		{name: "uint", kind: types.Uint, want: "strconv.FormatUint(uint64(v), 10)"},
		{name: "uint8", kind: types.Uint8, want: "strconv.FormatUint(uint64(v), 10)"},
		{name: "uint16", kind: types.Uint16, want: "strconv.FormatUint(uint64(v), 10)"},
		{name: "uint32", kind: types.Uint32, want: "strconv.FormatUint(uint64(v), 10)"},
		{name: "uint64", kind: types.Uint64, want: "strconv.FormatUint(v, 10)"},
		{name: "string", kind: types.String, want: "v"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			im := imports{}
			got, err := astgen.ConvertToString(im, ast.NewIdent("v"), source.NewType(types.Typ[tt.kind]))
			require.NoError(t, err)
			assert.Equal(t, tt.want, astgen.Format(got), "ConvertToString(%s)", tt.name)
		})
	}

	for _, tt := range []struct {
		name string
		tp   types.Type
		want string
	}{
		{name: "float", tp: types.Typ[types.Float64], want: "unsupported basic type for path parameters"},
		{name: "not basic", tp: types.NewSlice(types.Typ[types.Int]), want: "unsupported type for path parameters"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := astgen.ConvertToString(imports{}, ast.NewIdent("v"), source.NewType(tt.tp))
			assert.EqualError(t, err, tt.want, "ConvertToString(%s) error", tt.name)
		})
	}
}

func TestStrconvCalls(t *testing.T) {
	str := ast.NewIdent("s")
	for _, tt := range []struct {
		name string
		call func(astgen.ImportManager) ast.Expr
		want string
	}{
		{name: "Atoi", call: func(im astgen.ImportManager) ast.Expr { return astgen.StrconvAtoiCall(im, str) }, want: "strconv.Atoi(s)"},
		{name: "Itoa", call: func(im astgen.ImportManager) ast.Expr { return astgen.StrconvItoaCall(im, str) }, want: "strconv.Itoa(s)"},
		{name: "ParseBool", call: func(im astgen.ImportManager) ast.Expr { return astgen.StrconvParseBoolCall(im, str) }, want: "strconv.ParseBool(s)"},
		{name: "ParseInt", call: func(im astgen.ImportManager) ast.Expr { return astgen.StrconvParseIntCall(im, str, 10, 16) }, want: "strconv.ParseInt(s, 10, 16)"},
		{name: "ParseUint", call: func(im astgen.ImportManager) ast.Expr { return astgen.StrconvParseUintCall(im, str, 10, 0) }, want: "strconv.ParseUint(s, 10, 0)"},
		{name: "ParseFloat", call: func(im astgen.ImportManager) ast.Expr { return astgen.StrconvParseFloatCall(im, str, 32) }, want: "strconv.ParseFloat(s, 32)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			im := imports{}
			assert.Equal(t, tt.want, astgen.Format(tt.call(im)), "%s call", tt.name)
			assert.Equal(t, "strconv", im["strconv"], "%s should register the strconv import: %v", tt.name, im)
		})
	}
}

func TestBuilders(t *testing.T) {
	v := ast.NewIdent("v")
	for _, tt := range []struct {
		name    string
		node    func(astgen.ImportManager) ast.Node
		want    string
		imports []string
	}{
		{name: "Int", node: func(astgen.ImportManager) ast.Node { return astgen.Int(-3) }, want: "-3"},
		{name: "String quotes", node: func(astgen.ImportManager) ast.Node { return astgen.String(`a"b`) }, want: `"a\"b"`},
		{name: "Bool true", node: func(astgen.ImportManager) ast.Node { return astgen.Bool(true) }, want: "true"},
		{name: "Bool false", node: func(astgen.ImportManager) ast.Node { return astgen.Bool(false) }, want: "false"},
		{name: "Nil", node: func(astgen.ImportManager) ast.Node { return astgen.Nil() }, want: "nil"},
		{name: "EmptyStructType", node: func(astgen.ImportManager) ast.Node { return astgen.EmptyStructType() }, want: "struct {\n}"},
		{name: "CallBuiltin", node: func(astgen.ImportManager) ast.Node { return astgen.CallBuiltin("cap", v) }, want: "cap(v)"},
		{name: "CallBuiltinLen", node: func(astgen.ImportManager) ast.Node { return astgen.CallBuiltinLen(v) }, want: "len(v)"},
		{name: "CallBuiltinAppend", node: func(astgen.ImportManager) ast.Node { return astgen.CallBuiltinAppend(v, astgen.Int(1), astgen.Int(2)) }, want: "append(v, 1, 2)"},
		{name: "Convert", node: func(astgen.ImportManager) ast.Node {
			return astgen.Convert(&ast.ArrayType{Elt: ast.NewIdent("byte")}, v)
		}, want: "[]byte(v)"},
		{name: "ConvertIdent", node: func(astgen.ImportManager) ast.Node { return astgen.ConvertIdent("int64", v) }, want: "int64(v)"},
		{name: "CallError", node: func(astgen.ImportManager) ast.Node { return astgen.CallError("err") }, want: "err.Error()"},
		{name: "Call", node: func(im astgen.ImportManager) ast.Node { return astgen.Call(im, "", "path/filepath", "Join", v) }, want: "filepath.Join(v)", imports: []string{"path/filepath"}},
		{name: "Call alias", node: func(im astgen.ImportManager) ast.Node { return astgen.Call(im, "js", "encoding/json", "Marshal", v) }, want: "js.Marshal(v)", imports: []string{"encoding/json"}},
		{name: "CmpOr", node: func(im astgen.ImportManager) ast.Node { return astgen.CmpOr(im, v, astgen.String("/")) }, want: `cmp.Or(v, "/")`, imports: []string{"cmp"}},
		{name: "ErrorsNew", node: func(im astgen.ImportManager) ast.Node { return astgen.ErrorsNew(im, astgen.String("x")) }, want: `errors.New("x")`, imports: []string{"errors"}},
		{name: "ErrorsJoin", node: func(im astgen.ImportManager) ast.Node { return astgen.ErrorsJoin(im, v, v) }, want: "errors.Join(v, v)", imports: []string{"errors"}},
		{name: "SlogLoggerPtr", node: func(im astgen.ImportManager) ast.Node { return astgen.SlogLoggerPtr(im) }, want: "*slog.Logger", imports: []string{"log/slog"}},
		{name: "SlogString", node: func(im astgen.ImportManager) ast.Node { return astgen.SlogString(im, "k", v) }, want: `slog.String("k", v)`, imports: []string{"log/slog"}},
		{name: "ExportedIdentifier", node: func(im astgen.ImportManager) ast.Node { return astgen.ExportedIdentifier(im, "", "sync", "Mutex") }, want: "sync.Mutex", imports: []string{"sync"}},
		{name: "HTTPStatusCode known", node: func(im astgen.ImportManager) ast.Node { return astgen.HTTPStatusCode(im, 404) }, want: "http.StatusNotFound", imports: []string{"net/http"}},
		{name: "HTTPMethod known", node: func(im astgen.ImportManager) ast.Node { return astgen.HTTPMethod(im, "PATCH") }, want: "http.MethodPatch", imports: []string{"net/http"}},
		{name: "HTTPMethod unknown", node: func(im astgen.ImportManager) ast.Node { return astgen.HTTPMethod(im, "PURGE") }, want: `"PURGE"`},
		{name: "HTTPMethod empty", node: func(im astgen.ImportManager) ast.Node { return astgen.HTTPMethod(im, "") }, want: `""`},
		{name: "HTTPStatusCode unknown", node: func(im astgen.ImportManager) ast.Node { return astgen.HTTPStatusCode(im, 299) }, want: "299"},
		{name: "HTTPErrorCall", node: func(im astgen.ImportManager) ast.Node {
			return astgen.HTTPErrorCall(im, ast.NewIdent("w"), astgen.String("no"), 400)
		}, want: `http.Error(w, "no", http.StatusBadRequest)`, imports: []string{"net/http"}},
		{name: "HTTPRequestPtr", node: func(im astgen.ImportManager) ast.Node { return astgen.HTTPRequestPtr(im) }, want: "*http.Request", imports: []string{"net/http"}},
		{name: "HTTPResponseWriter", node: func(im astgen.ImportManager) ast.Node { return astgen.HTTPResponseWriter(im) }, want: "http.ResponseWriter", imports: []string{"net/http"}},
		{name: "HTTPHandlerFuncType", node: func(im astgen.ImportManager) ast.Node {
			return assignBlank(&ast.FuncLit{Type: astgen.HTTPHandlerFuncType(im, "w", "r"), Body: &ast.BlockStmt{}})
		}, want: "_ = func(w http.ResponseWriter, r *http.Request) {\n}", imports: []string{"net/http"}},
		{name: "HTTPMiddlewareFuncType", node: func(im astgen.ImportManager) ast.Node {
			return assignBlank(&ast.FuncLit{Type: astgen.HTTPMiddlewareFuncType(im), Body: &ast.BlockStmt{}})
		}, want: "_ = func(next http.Handler) http.Handler {\n}", imports: []string{"net/http"}},
		{name: "SyncPoolBytesBuffer", node: func(im astgen.ImportManager) ast.Node { return astgen.SyncPoolBytesBuffer(im) }, want: "sync.Pool{New: func() any {\n\treturn bytes.NewBuffer(nil)\n}}", imports: []string{"sync", "bytes"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			im := imports{}
			assert.Equal(t, tt.want, astgen.Format(tt.node(im)), tt.name)
			for _, pkg := range tt.imports {
				assert.Contains(t, im, pkg, "%s should register the %s import", tt.name, pkg)
			}
			assert.Len(t, im, len(tt.imports), "%s registered imports %v, want %v", tt.name, im, tt.imports)
		})
	}
}

func TestGetBufferFromPool(t *testing.T) {
	stmts := astgen.GetBufferFromPool(imports{}, "pool", "buf")
	want := "{\n\tbuf := pool.Get().(*bytes.Buffer)\n\tbuf.Reset()\n\tdefer pool.Put(buf)\n}"
	assert.Equal(t, want, astgen.Format(&ast.BlockStmt{List: stmts}), "GetBufferFromPool")
}

func TestFormatReportsUnparsableNodes(t *testing.T) {
	got := astgen.Format(&ast.BasicLit{Kind: token.INT, Value: "not a number"})
	assert.True(t, strings.HasPrefix(got, "formatting error:"), "Format(bad literal) = %q, want it to report a formatting error", got)
}

func newPackage(path, name string) *types.Package {
	return types.NewPackage(path, name)
}

// TestTypeFormatter states how a generated file names the packages its types
// come from. Qualifier remembers the name it gave each import path, so the
// subtests that depend on that build their own formatter and say so.
func TestTypeFormatter(t *testing.T) {
	const outputPath = "example.com/server"

	// newFormatter writes for the output package with the identifiers the
	// file already declares.
	newFormatter := func(idents ...string) *astgen.TypeFormatter {
		tf := astgen.NewTypeFormatter(outputPath)
		tf.Idents = idents
		return tf
	}
	// collectedFormatter has already been asked about three import paths,
	// two of which share the name other.
	collectedFormatter := func() *astgen.TypeFormatter {
		tf := newFormatter()
		tf.Imports = map[string]string{
			"example.com/x/other": "other",
			"example.com/third":   "third3",
			"example.com/other":   "other1",
		}
		return tf
	}

	t.Run("a nil package is qualified by the empty string", func(t *testing.T) {
		tf := newFormatter("server")
		assert.Empty(t, tf.Qualifier(nil), "Qualifier(nil)")
	})

	t.Run("the output package is qualified by the empty string", func(t *testing.T) {
		tf := newFormatter("server")
		assert.Empty(t, tf.Qualifier(newPackage(outputPath, "server")), "Qualifier(output package)")
	})

	t.Run("a name the file declares gets the next free numeric suffix", func(t *testing.T) {
		tf := newFormatter("other")
		assert.Equal(t, "other1", tf.Qualifier(newPackage("example.com/other", "other")), "Qualifier(other) because the file declares other")
	})

	t.Run("a package keeps the name it was given", func(t *testing.T) {
		other := newPackage("example.com/other", "other")
		tf := newFormatter("other")
		require.Equal(t, "other1", tf.Qualifier(other), "Qualifier(other)")
		assert.Equal(t, "other1", tf.Qualifier(other), "Qualifier(other) again with the same declared names")

		tf.Idents = nil
		assert.Equal(t, "other1", tf.Qualifier(other), "Qualifier(other) after the file stops declaring other")
	})

	t.Run("the suffix skips every declared name", func(t *testing.T) {
		tf := newFormatter("third", "third1", "third2")
		assert.Equal(t, "third3", tf.Qualifier(newPackage("example.com/third", "third")), "Qualifier(third), the first name the file does not declare")
	})

	t.Run("two packages with one name are told apart by import path", func(t *testing.T) {
		tf := newFormatter("other")
		require.Equal(t, "other1", tf.Qualifier(newPackage("example.com/other", "other")), "Qualifier(example.com/other)")

		tf.Idents = nil
		assert.Equal(t, "other", tf.Qualifier(newPackage("example.com/x/other", "other")), "Qualifier(example.com/x/other)")
		assert.Equal(t, map[string]string{
			"example.com/other":   "other1",
			"example.com/x/other": "other",
		}, tf.Imports, "each import path keeps its own name")
	})

	t.Run("GenDecl lists imports sorted by path and omits an alias equal to the last path element", func(t *testing.T) {
		want := "import (\n\tother1 \"example.com/other\"\n\tthird3 \"example.com/third\"\n\t\"example.com/x/other\"\n)"
		assert.Equal(t, want, astgen.Format(collectedFormatter().GenDecl()), "GenDecl")
	})

	t.Run("JSON marshals the path to name map with deterministic key order", func(t *testing.T) {
		encoded, err := json.Marshal(collectedFormatter(), json.Deterministic(true))
		require.NoError(t, err)
		want := `{"example.com/other":"other1","example.com/third":"third3","example.com/x/other":"other"}`
		assert.Equal(t, want, string(encoded), "json.Marshal(TypeFormatter)")
	})
}
