package astgen_test

import (
	"encoding/json/v2"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"
	"testing"

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
			if err != nil {
				t.Fatal(err)
			}
			if s := astgen.Format(got); s != tt.want {
				t.Errorf("ConvertToString(%s) = %q, want %q", tt.name, s, tt.want)
			}
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
			if err == nil || err.Error() != tt.want {
				t.Errorf("ConvertToString(%s) error = %v, want %q", tt.name, err, tt.want)
			}
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
			if got := astgen.Format(tt.call(im)); got != tt.want {
				t.Errorf("%s call = %q, want %q", tt.name, got, tt.want)
			}
			if im["strconv"] != "strconv" {
				t.Errorf("%s did not register the strconv import: %v", tt.name, im)
			}
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
			if got := astgen.Format(tt.node(im)); got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
			}
			for _, pkg := range tt.imports {
				if _, ok := im[pkg]; !ok {
					t.Errorf("%s did not register the %s import: %v", tt.name, pkg, im)
				}
			}
			if len(im) != len(tt.imports) {
				t.Errorf("%s registered imports %v, want %v", tt.name, im, tt.imports)
			}
		})
	}
}

func TestGetBufferFromPool(t *testing.T) {
	stmts := astgen.GetBufferFromPool(imports{}, "pool", "buf")
	want := "{\n\tbuf := pool.Get().(*bytes.Buffer)\n\tbuf.Reset()\n\tdefer pool.Put(buf)\n}"
	if got := astgen.Format(&ast.BlockStmt{List: stmts}); got != want {
		t.Errorf("GetBufferFromPool = %q, want %q", got, want)
	}
}

func TestFindFieldWithName(t *testing.T) {
	list := &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{ast.NewIdent("a"), ast.NewIdent("b")}, Type: ast.NewIdent("int")},
		{Names: []*ast.Ident{ast.NewIdent("c")}, Type: ast.NewIdent("string")},
	}}
	for _, tt := range []struct {
		name     string
		wantType string
		wantOK   bool
	}{
		{name: "a", wantType: "int", wantOK: true},
		{name: "b", wantType: "int", wantOK: true},
		{name: "c", wantType: "string", wantOK: true},
		{name: "d"},
	} {
		field, ok := astgen.FindFieldWithName(list, tt.name)
		if ok != tt.wantOK {
			t.Errorf("FindFieldWithName(%q) found = %v, want %v", tt.name, ok, tt.wantOK)
			continue
		}
		if ok && astgen.Format(field.Type) != tt.wantType {
			t.Errorf("FindFieldWithName(%q).Type = %s, want %s", tt.name, astgen.Format(field.Type), tt.wantType)
		}
	}
}

func TestFormatReportsUnparsableNodes(t *testing.T) {
	got := astgen.Format(&ast.BasicLit{Kind: token.INT, Value: "not a number"})
	if !strings.HasPrefix(got, "formatting error:") {
		t.Errorf("Format(bad literal) = %q, want it to report a formatting error", got)
	}
}

func TestTypeFormatter(t *testing.T) {
	server := types.NewPackage("example.com/server", "server")
	other := types.NewPackage("example.com/other", "other")
	colliding := types.NewPackage("example.com/x/other", "other")

	tf := astgen.NewTypeFormatter(server.Path())
	tf.Idents = []string{"other"}

	if got := tf.Qualifier(nil); got != "" {
		t.Errorf("Qualifier(nil) = %q, want empty", got)
	}
	if got := tf.Qualifier(server); got != "" {
		t.Errorf("Qualifier(output package) = %q, want empty", got)
	}
	if got := tf.Qualifier(other); got != "other1" {
		t.Errorf("Qualifier(other) = %q, want other1 because the file declares other", got)
	}
	if got := tf.Qualifier(other); got != "other1" {
		t.Errorf("Qualifier(other) again = %q, want the name it was given", got)
	}
	tf.Idents = nil
	if got := tf.Qualifier(colliding); got != "other" {
		t.Errorf("Qualifier(colliding) = %q, want other", got)
	}

	want := "import (\n\tother1 \"example.com/other\"\n\t\"example.com/x/other\"\n)"
	if got := astgen.Format(tf.GenDecl()); got != want {
		t.Errorf("GenDecl = %q, want %q", got, want)
	}

	encoded, err := json.Marshal(tf, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"example.com/other":"other1","example.com/x/other":"other"}`; string(encoded) != want {
		t.Errorf("json.Marshal(TypeFormatter) = %s, want %s", encoded, want)
	}
}
