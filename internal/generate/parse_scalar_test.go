package generate

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

func scalarTestFile(t *testing.T) *File {
	t.Helper()
	pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": "package server\n\ntype ID int\n"})
	return newFile(source.Package{Fset: fake.FileSet, Types: pkg})
}

func TestScalarParseStatements(t *testing.T) {
	errBlock := func() *ast.BlockStmt {
		return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{}}}
	}
	validation := func() []ast.Stmt {
		return []ast.Stmt{&ast.ExprStmt{X: ast.NewIdent("validate")}}
	}
	assign := singleAssignment(token.ASSIGN, ast.NewIdent("out"))

	basic := func(kind types.BasicKind) source.Type { return source.NewType(types.Typ[kind]) }
	id := func(t *testing.T) source.Type {
		return source.NewType(fake.Lookup(t, scalarTestFile(t).pkg.Types, "ID"))
	}

	for _, tt := range []struct {
		name        string
		method      muxt.UnmarshalMethod
		typ         func(*testing.T) source.Type
		validations []ast.Stmt
		want        string
	}{
		{
			name: "bool", method: muxt.UnmarshalBool,
			typ: func(*testing.T) source.Type { return basic(types.Bool) },
			want: `tmp, err := strconv.ParseBool(str)
if err != nil {
	return
}
out = tmp`,
		},
		{
			name: "int", method: muxt.UnmarshalInt,
			typ: func(*testing.T) source.Type { return basic(types.Int) },
			want: `tmp, err := strconv.Atoi(str)
if err != nil {
	return
}
out = tmp`,
		},
		{
			name: "int8", method: muxt.UnmarshalInt8,
			typ: func(*testing.T) source.Type { return basic(types.Int8) },
			want: `tmp, err := strconv.ParseInt(str, 10, 8)
if err != nil {
	return
}
out = int8(tmp)`,
		},
		{
			name: "int16", method: muxt.UnmarshalInt16,
			typ: func(*testing.T) source.Type { return basic(types.Int16) },
			want: `tmp, err := strconv.ParseInt(str, 10, 16)
if err != nil {
	return
}
out = int16(tmp)`,
		},
		{
			name: "int32", method: muxt.UnmarshalInt32,
			typ: func(*testing.T) source.Type { return basic(types.Int32) },
			want: `tmp, err := strconv.ParseInt(str, 10, 32)
if err != nil {
	return
}
out = int32(tmp)`,
		},
		{
			name: "int64", method: muxt.UnmarshalInt64,
			typ: func(*testing.T) source.Type { return basic(types.Int64) },
			want: `tmp, err := strconv.ParseInt(str, 10, 64)
if err != nil {
	return
}
out = tmp`,
		},
		{
			name: "uint", method: muxt.UnmarshalUint,
			typ: func(*testing.T) source.Type { return basic(types.Uint) },
			want: `tmp, err := strconv.ParseUint(str, 10, 0)
if err != nil {
	return
}
out = uint(tmp)`,
		},
		{
			name: "uint8", method: muxt.UnmarshalUint8,
			typ: func(*testing.T) source.Type { return basic(types.Uint8) },
			want: `tmp, err := strconv.ParseUint(str, 10, 8)
if err != nil {
	return
}
out = uint8(tmp)`,
		},
		{
			name: "uint16", method: muxt.UnmarshalUint16,
			typ: func(*testing.T) source.Type { return basic(types.Uint16) },
			want: `tmp, err := strconv.ParseUint(str, 10, 16)
if err != nil {
	return
}
out = uint16(tmp)`,
		},
		{
			name: "uint32", method: muxt.UnmarshalUint32,
			typ: func(*testing.T) source.Type { return basic(types.Uint32) },
			want: `tmp, err := strconv.ParseUint(str, 10, 32)
if err != nil {
	return
}
out = uint32(tmp)`,
		},
		{
			name: "uint64", method: muxt.UnmarshalUint64,
			typ: func(*testing.T) source.Type { return basic(types.Uint64) },
			want: `tmp, err := strconv.ParseUint(str, 10, 64)
if err != nil {
	return
}
out = tmp`,
		},
		{
			name: "float32", method: muxt.UnmarshalFloat32,
			typ: func(*testing.T) source.Type { return basic(types.Float32) },
			want: `tmp, err := strconv.ParseFloat(str, 32)
if err != nil {
	return
}
out = float32(tmp)`,
		},
		{
			name: "float64", method: muxt.UnmarshalFloat64,
			typ: func(*testing.T) source.Type { return basic(types.Float64) },
			want: `tmp, err := strconv.ParseFloat(str, 64)
if err != nil {
	return
}
out = tmp`,
		},
		{
			name: "string", method: muxt.UnmarshalString,
			typ:  func(*testing.T) source.Type { return basic(types.String) },
			want: `out = str`,
		},
		{
			name: "string with validations", method: muxt.UnmarshalString,
			typ:         func(*testing.T) source.Type { return basic(types.String) },
			validations: validation(),
			want: `tmp := str
validate
out = tmp`,
		},
		{
			name: "int with validations", method: muxt.UnmarshalInt,
			typ:         func(*testing.T) source.Type { return basic(types.Int) },
			validations: validation(),
			want: `tmp, err := strconv.Atoi(str)
if err != nil {
	return
} else {
	validate
}
out = tmp`,
		},
		{
			name: "text unmarshaler", method: muxt.UnmarshalTextUnmarshaler,
			typ: id,
			want: `var tmp ID
if err := tmp.UnmarshalText([]byte(str)); err != nil {
	return
}
out = tmp`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := scalarTestFile(t)
			stmts, err := scalarParse{
				tmp: "tmp", str: ast.NewIdent("str"), typ: tt.typ(t), method: tt.method,
				validations: tt.validations, assign: assign, errBlock: errBlock(),
			}.statements(file)
			require.NoError(t, err)
			got := strings.TrimSpace(astgen.Format(&ast.BlockStmt{List: stmts}))
			got = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(got, "{"), "}"))
			got = strings.ReplaceAll(got, "\n\t", "\n")
			assert.Equal(t, tt.want, got, "scalarParse(%s).statements", tt.name)
		})
	}

	t.Run("unsupported", func(t *testing.T) {
		_, err := scalarParse{
			tmp: "tmp", str: ast.NewIdent("str"), typ: basic(types.Complex128), method: muxt.UnmarshalUnsupported,
			assign: assign, errBlock: errBlock(),
		}.statements(scalarTestFile(t))
		assert.ErrorContains(t, err, "unsupported type: complex128")
	})
}
