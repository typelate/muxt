package check_test

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/types"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

const packageName = "check_test"

var loadPkg = sync.OnceValue(func() []*packages.Package {
	packageList, loadErr := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedSyntax | packages.NeedFiles | packages.NeedDeps | packages.NeedTypes,
		Tests: true,
	}, ".")
	if loadErr != nil {
		panic(loadErr)
	}
	return packageList
})

// functionSignatures returns a map of function name -> *types.Signature for the given function names.
func functionSignatures(t *testing.T, pkg *packages.Package, names ...string) map[string]*types.Signature {
	t.Helper()
	sigs := make(map[string]*types.Signature)
	for _, name := range names {
		fn := pkg.Types.Scope().Lookup(name)
		require.NotNil(t, fn, "function %q not found in package scope", name)
		sig, ok := fn.Type().(*types.Signature)
		require.True(t, ok, "function %q is not a *types.Signature", name)
		sigs[name] = sig
	}
	return sigs
}

func TestTree(t *testing.T) {
	const testFuncName = "TestTree"
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})

	_, currentFileName, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to find current file name")
	fileIndex := slices.IndexFunc(testPkg.Syntax, func(file *ast.File) bool {
		pos := testPkg.Fset.Position(file.Pos())
		return pos.Filename == currentFileName
	})
	if fileIndex < 0 {
		t.Fatal("no check_test.go found")
	}
	file := testPkg.Syntax[fileIndex]

	type ttRow struct {
		Name     string
		Template string
		Data     any
		Error    func(t *testing.T, checkErr error, tp types.Type)
	}

	var ttRows *ast.CompositeLit
	for _, decl := range file.Decls {
		testFunc, ok := decl.(*ast.FuncDecl)
		if !ok || testFunc.Name.Name != testFuncName {
			continue
		}
		for _, stmt := range testFunc.Body.List {
			rangeStatement, ok := stmt.(*ast.RangeStmt)
			if !ok {
				continue
			}
			tests, ok := rangeStatement.X.(*ast.CompositeLit)
			if !ok {
				continue
			}
			arr, ok := tests.Type.(*ast.ArrayType)
			if !ok {
				continue
			}

			if testType, ok := arr.Elt.(*ast.Ident); !ok || testType.Name != "ttRow" {
				continue
			}
			ttRows = tests
		}
	}

	funcNames := []string{
		"square", "ceil",
		"expectInt", "expectFloat64", "expectString",
		"expectInt8", "expectInt16", "expectInt32", "expectInt64",
		"expectUint", "expectUint8", "expectUint16", "expectUint32", "expectUint64",
		"expectFloat32", "expectComplex64", "expectComplex128",
		"expectBool",
		"typeOf", "stringer", "echoT",
	}

	for _, tt := range []ttRow{
		{
			Name:     "when accessing nil on an empty struct",
			Template: `{{.Field}}`,
			Data:     Void{},
			Error: func(t *testing.T, err error, tp types.Type) {
				require.ErrorContains(t, err, "Field not found on")
			},
		},
		{
			Name:     "when accessing the dot",
			Template: `{{.}}`,
			Data:     Void{},
		},
		{
			Name:     "when a method does not any results",
			Template: `{{.Method}}`,
			Data:     TypeWithMethodSignatureNoResultMethod{},
			Error: func(t *testing.T, err error, tp types.Type) {
				require.ErrorContains(t, err, "function Method has 0 return values; should be 1 or 2")
			},
		},
		{
			Name:     "when a method has a result",
			Template: `{{.Method}}`,
			Data:     TypeWithMethodSignatureResult{},
		},
		{
			Name:     "when a method also has an error",
			Template: `{{.Method}}`,
			Data:     TypeWithMethodSignatureResultAndError{},
		},
		{
			Name:     "when a method has a second result that is not an error",
			Template: `{{.Method}}`,
			Data:     TypeWithMethodSignatureResultAndNonError{},
			Error: func(t *testing.T, err error, tp types.Type) {
				require.ErrorContains(t, err, "invalid function signature for Method: second return value should be error; is int")
			},
		},
		{
			Name:     "when a method with too many results",
			Template: `{{.Method}}`,
			Data:     TypeWithMethodSignatureThreeResults{},
			Error: func(t *testing.T, err error, tp types.Type) {
				require.ErrorContains(t, err, "function Method has 3 return values; should be 1 or 2")
			},
		},
		{
			Name:     "when a method is part of a field node list",
			Template: `{{.Method.Method}}`,
			Data:     TypeWithMethodSignatureResultHasMethod{},
		},
		{
			Name:     "when result method does not have a method",
			Template: `{{.Method.Method}}`,
			Data:     TypeWithMethodSignatureResultHasMethodWithNoResults{},
			Error: func(t *testing.T, err error, tp types.Type) {
				require.ErrorContains(t, err, "function Method has 0 return values; should be 1 or 2")
			},
		},
		{
			Name:     "when the struct has the field",
			Template: `{{.Field}}`,
			Data:     StructWithField{},
		},
		{
			Name:     "when the struct has the field and the field has a method",
			Template: `{{.Field.Method}}`,
			Data:     StructWithFieldWithMethod{},
		},
		{
			Name:     "when the struct has the field and the field has a method (field only)",
			Template: `{{.Field}}`,
			Data:     StructWithFieldWithMethod{},
		},
		{
			Name:     "when the struct has the field of kind func",
			Template: `{{.Func.Method}}`,
			Data: StructWithFuncFieldWithResultWithMethod{
				Func: func() (_ TypeWithMethodSignatureResult) { return },
			},
			Error: func(t *testing.T, err error, tp types.Type) {
				require.ErrorContains(t, err, "identifier chain not supported for type")
			},
		},
		{
			Name:     "when a method has an int parameter",
			Template: `{{.F 21}}`,
			Data:     MethodWithIntParam{},
		},
		{
			Name:     "when a method argument is a bool but param is int",
			Template: `{{.F false}}`,
			Data:     MethodWithIntParam{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.Error(t, checkErr)
				require.ErrorContains(t, checkErr, "expected int")
			},
		},
		{
			Name:     "a method that takes an argument is called with none in the middle of a chain",
			Template: `{{.Get.Field}}`,
			Data:     MethodsInChain{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "wrong number of args expected 1 but got 0")
			},
		},
		{
			Name:     "a variadic method needs no argument in the middle of a chain",
			Template: `{{.Variadic.Field}}`,
			Data:     MethodsInChain{},
		},
		{
			Name:     "not found shows available field",
			Template: `{{.Missing}}`,
			Data:     StructWithField{},
			Error: func(t *testing.T, err error, tp types.Type) {
				require.ErrorContains(t, err, "Missing not found on")
				require.Contains(t, detailedError(t, err), "Field struct{}")
			},
		},
		{
			Name:     "when the struct has the field and the field has a method",
			Template: `{{.Field.Method}}`,
			Data:     StructWithFieldWithMethod{},
		},
		{
			Name:     "when a method has a bool parameter",
			Template: `{{.F true}}`,
			Data:     MethodWithBoolParam{},
		},
		{
			Name:     "when a method argument is an int but param is bool",
			Template: `{{.F 32}}`,
			Data:     MethodWithBoolParam{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.Error(t, checkErr)
				require.ErrorContains(t, checkErr, "expected bool")
			},
		},
		{
			Name:     "when a method receives a 64 bit floating point literal",
			Template: `{{.F 3.2}}`,
			Data:     MethodWithFloat64Param{},
		},
		{
			Name:     "when a method receives a 32 bit floating point literal",
			Template: `{{.F 3.2}}`,
			Data:     MethodWithFloat32Param{},
		},
		{
			Name:     "when the method parameter is an int8",
			Template: `{{.F 32}}`,
			Data:     MethodWithInt8Param{},
		},
		{
			Name:     "when the method parameter is an int16",
			Template: `{{.F 32}}`,
			Data:     MethodWithInt16Param{},
		},
		{
			Name:     "when the method parameter is an int32",
			Template: `{{.F 32}}`,
			Data:     MethodWithInt32Param{},
		},
		{
			Name:     "when the method parameter is an int64",
			Template: `{{.F 32}}`,
			Data:     MethodWithInt64Param{},
		},
		{
			Name:     "when the method parameter is an uint",
			Template: `{{.F 32}}`,
			Data:     MethodWithUintParam{},
		},
		{
			Name:     "when the method parameter is an uint8",
			Template: `{{.F 32}}`,
			Data:     MethodWithUint8Param{},
		},
		{
			Name:     "when the method parameter is an uint16",
			Template: `{{.F 32}}`,
			Data:     MethodWithUint16Param{},
		},
		{
			Name:     "when the method parameter is an uint32",
			Template: `{{.F 32}}`,
			Data:     MethodWithUint32Param{},
		},
		{
			Name:     "when the method parameter is an uint64",
			Template: `{{.F 32}}`,
			Data:     MethodWithUint64Param{},
		},
		{
			Name:     "when a method is on the dollar variable",
			Template: `{{$.F 32}}`,
			Data:     MethodWithUint64Param{},
		},
		{
			Name:     "when accessing the dollar variable in an underlying template",
			Template: `{{define "t1"}}{{$.F 3.2}}{{end}}{{template "t1" $.Method}}`,
			Data:     TypeWithMethodSignatureResultMethodWithFloat32Param{},
		},
		{
			Name:     "when ranging over a slice field",
			Template: `{{range .Numbers}}{{$.F .}}{{end}}`,
			Data: TypeWithMethodAndSliceFloat64{
				Numbers: []float64{1, 2, 3},
			},
		},
		{
			Name:     "when ranging over an array field",
			Template: `{{range .Numbers}}{{$.F .}}{{end}}`,
			Data: TypeWithMethodAndArrayFloat64{
				Numbers: [...]float64{1, 2},
			},
		},
		{
			Name:     "when passing key value range variables for slice",
			Template: `{{range $k, $v := .Numbers}}{{$.F $k $v}}{{end}}`,
			Data: MethodWithKeyValForSlices{
				Numbers: []float64{1, 2},
			},
		},
		{
			Name:     "when passing key value range variables for slice from variable",
			Template: `{{with $slice := .Numbers}}{{range $k, $v := $slice}}{{$.F $k $v}}{{end}}{{end}}`,
			Data: MethodWithKeyValForSlices{
				Numbers: []float64{1, 2},
			},
		},
		{
			Name:     "when passing key value range variables for array",
			Template: `{{range $k, $v := .Numbers}}{{$.F $k $v}}{{end}}`,
			Data: MethodWithKeyValForArray{
				Numbers: [...]float64{1, 2},
			},
		},
		{
			Name:     "when passing key value range variables for map",
			Template: `{{range $k, $v := .Numbers}}{{$.F $k $v}}{{end}}`,
			Data: MethodWithKeyValForMap{
				Numbers: map[int16]float32{},
			},
		},
		{
			Name:     "when iter1 method",
			Template: `{{range .Method}}{{expectInt8 .}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when named iter1 method",
			Template: `{{range $v := .Method}}{{expectInt8 $v}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when iter2 method",
			Template: `{{range .Method2}}{{expectInt8 .}}{{expectInt8 .}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when named iter2 method",
			Template: `{{range $v := .Method2}}{{expectInt8 $v}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when iter1 field",
			Template: `{{range .Field}}{{expectInt8 .}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when named iter1 field",
			Template: `{{range $v := .Field}}{{expectInt8 $v}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when iter2 field",
			Template: `{{range .Field2}}{{expectInt8 .}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when named iter2 field",
			Template: `{{range $v := .Field2}}{{expectInt8 $v}}{{expectInt8 .}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when named key value iter2 method",
			Template: `{{range $k := .Method}}{{expectInt8 $k}}{{expectInt8 .}}{expectFloat64 $v}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "when named key value iter2 field",
			Template: `{{range $k, $v := .Field2}}{{expectInt8 $k}}{{expectFloat64 .}}{{expectFloat64 $v}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "range over too many variables",
			Template: `{{range $k, $v := .Method}}{{expectInt8 $k}}{{expectFloat64 .}}{expectFloat64 $v}{{end}}`,
			Data:     NewIterators(),
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "iterate over more than one variable")
			},
		},
		{
			Name:     "range over int literal",
			Template: `{{range 10}}{{expectInt .}}{{end}}`,
			Data:     T{},
		},
		{
			Name:     "range over int field",
			Template: `{{range .I}}{{expectInt .}}{{end}}`,
			Data:     T{},
		},
		{
			Name:     "range over int data",
			Template: `{{range .}}{{expectInt .}}{{end}}`,
			Data:     int(32),
		},
		{
			Name:     "range over int8 data",
			Template: `{{range .}}{{expectInt8 .}}{{end}}`,
			Data:     int8(32),
		},
		{
			Name:     "range over int16 data",
			Template: `{{range .}}{{expectInt16 .}}{{end}}`,
			Data:     int16(32),
		},
		{
			Name:     "range over int32 data",
			Template: `{{range .}}{{expectInt32 .}}{{end}}`,
			Data:     int32(32),
		},
		{
			Name:     "range over int64 data",
			Template: `{{range .}}{{expectInt64 .}}{{end}}`,
			Data:     int64(32),
		},
		{
			Name:     "range over uint data",
			Template: `{{range .}}{{expectUint .}}{{end}}`,
			Data:     uint(32),
		},
		{
			Name:     "range over uint8 data",
			Template: `{{range .}}{{expectUint8 .}}{{end}}`,
			Data:     uint8(32),
		},
		{
			Name:     "range over uint16 data",
			Template: `{{range .}}{{expectUint16 .}}{{end}}`,
			Data:     uint16(32),
		},
		{
			Name:     "range over uint32 data",
			Template: `{{range .}}{{expectUint32 .}}{{end}}`,
			Data:     uint32(32),
		},
		{
			Name:     "range over uint64 data",
			Template: `{{range .}}{{expectUint64 .}}{{end}}`,
			Data:     uint64(32),
		},
		{
			Name:     "range over float64 data",
			Template: `{{range .}}{{expectUint64 .}}{{end}}`,
			Data:     float64(32),
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "range can't iterate over float64")
			},
		},
		{
			Name:     "range over string data",
			Template: `{{range .}}{{end}}`,
			Data:     "fail",
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "range can't iterate over string")
			},
		},
		{
			Name:     "when a variable is used",
			Template: `{{$v := 1}}{{.F $v}}`,
			Data:     MethodWithIntParam{},
		},
		{
			Name:     "when there is an error in the else block",
			Template: `{{$x := "wrong type"}}{{if false}}{{else}}{{.F $x}}{{end}}`,
			Data:     MethodWithIntParam{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.Error(t, checkErr)
				require.ErrorContains(t, checkErr, "argument 0 has type untyped string expected int")
			},
		},
		{
			Name:     "variable redefined in if block",
			Template: `{{$x := 1}}{{if true}}{{$x := "str"}}{{end}}{{.F $x}}`,
			Data:     MethodWithIntParam{},
		},
		{
			Name:     "range variable does not clobber outer scope",
			Template: `{{$x := 1}}{{range .Numbers}}{{$x := "str"}}{{end}}{{square $x}}`,
			Data:     MethodWithKeyValForSlices{},
		},
		{
			Name:     "range variable does not override outer scope",
			Template: `{{$x := "str"}}{{range $x, $y := .Numbers}}{{$.F $x $y}}{{end}}{{printf $x}}`,
			Data:     MethodWithKeyValForSlices{},
		},
		{
			Name:     "source provided function",
			Template: `{{square 5}}`,
			Data:     Void{},
		},
		{
			Name:     "with expression",
			Template: `{{$x := 1}}{{with $x := .Numbers}}{{$x}}{{end}}`,
			Data:     MethodWithKeyValForSlices{},
		},
		{
			Name:     "with expression declares variable with same name as parent scope",
			Template: `{{$x := 1.2}}{{with $x := ceil $x}}{{$x}}{{end}}`,
			Data:     MethodWithKeyValForSlices{},
		},
		{
			Name:     "with expression has action with wrong dot type used in call",
			Template: `{{with $x := "wrong"}}{{expectInt .}}{{else}}{{end}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "argument 0 has type untyped string expected int")
			},
		},
		{
			Name:     "with else expression has action with correct dot type used in call",
			Template: `{{with $x := 12}}{{with $x := 1.2}}{{else}}{{expectInt $x}}{{end}}{{end}}`,
			Data:     Void{},
		},
		{
			Name:     "with else expression has action with wrong dot type used in call",
			Template: `{{with $outer := 12}}{{with $x := true}}{{else}}{{expectString .}}{{end}}{{end}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "argument 0 has type untyped int expected string")
			},
		},
		{
			Name:     "complex number parses",
			Template: `{{$x := 2i}}{{printf "%T" $x}}`,
			Data:     Void{},
		},
		{
			Name:     "template node without parameter",
			Template: `{{define "t"}}{{end}}{{template "t"}}`,
			Data:     Void{},
		},
		{
			Name:     "template wrong input type",
			Template: `{{define "t"}}{{expectInt .}}{{end}}{{if false}}{{template "t" 1.2}}{{end}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "argument 0 has type float64 expected int")
			},
		},
		{
			Name:     "it downgrades untyped integers",
			Template: `{{define "t"}}{{expectInt8 .}}{{end}}{{if false}}{{template "t" 12}}{{end}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "argument 0 has type int expected int8")
			},
		},
		{
			Name:     "chain node",
			Template: `{{(.).A.B.C.D}}`,
			Data:     LetterChainA{},
		},
		{
			Name:     "chain node with type change in term",
			Template: `{{(.A).B.C.D}}`,
			Data:     LetterChainA{},
		},
		{
			Name:     "empty",
			Template: "",
			Data:     Void{},
		},
		{
			Name:     "text",
			Template: "some text",
			Data:     Void{},
		},
		{
			Name:     "nil action",
			Template: `{{nil}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "nil is not a command")
			},
		},
		{
			Name:     "ideal int",
			Template: `{{expectInt 3}}`,
			Data:     Void{},
		},
		{
			Name:     "ideal float",
			Template: `{{expectFloat64 1.0}}`,
			Data:     Void{},
		},
		{
			Name:     "ideal exponent",
			Template: `{{expectFloat64 1e1}}`,
			Data:     Void{},
		},
		{
			Name:     "ideal complex",
			Template: `{{expectComplex128 1i}}`,
			Data:     Void{},
		},
		{
			Name:     ".X",
			Template: "-{{.X}}-",
			Data:     tVal,
		},
		{
			Name:     ".U.V",
			Template: "-{{.U.V}}-",
			Data:     tVal,
		},
		{
			Name:     ".unexported",
			Template: "{{.unexported}}",
			Data:     tVal,
			Error: func(t *testing.T, checkErr error, _ types.Type) {
				require.Error(t, checkErr)
			},
		},
		{
			Name:     "Interface call",
			Template: `{{stringer .S}}`,
			Data: map[string]fmt.Stringer{
				"S": bytes.NewBufferString("foozle"),
			},
		},
		{
			Name:     "error method, error",
			Template: "{{.MyError true}}",
			Data:     tVal,
		},
		{
			Name:     "nil call arg",
			Template: `{{ call .TVal.NilOKFunc .NilInt }}`,
			Data: &struct {
				TVal   *T
				NilInt *int
			}{
				TVal: tVal,
			},
		},
		{
			Name:     "len of an interface field",
			Template: "{{len .Empty3}}",
			Data:     tVal,
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "built-in len expects the first argument to be an array, slice, map, or string got any")
			},
		},
		{
			Name:     "and undef",
			Template: "{{and 1 .Unknown}}",
			Data:     nil,
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "Unknown not found on untyped nil")
			},
		},
		{
			Name:     "or undef",
			Template: "{{or 0 .Unknown}}",
			Data:     nil,
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "Unknown not found on untyped nil")
			},
		},
		{
			Name:     "nil pipeline",
			Template: "{{ .NilInt | call .NilOKFunc }}",
			Data: struct {
				*T
				NilInt *int
			}{
				T: tVal,
			},
		},
		{
			Name:     "parens: $ in paren in pipe",
			Template: "{{($ | echoT).X}}",
			Data:     tVal,
		},
		{
			Name:     "pointer param with wrong second arg",
			Template: `{{.F .I 42}}`,
			Data:     MethodWithPointerIntAndString{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "expected string")
			},
		},
		{
			Name:     "range int body is checked",
			Template: `{{range 5}}{{.BadField}}{{end}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "BadField not found")
			},
		},
		{
			Name:     "range iter body is checked",
			Template: `{{range .Method}}{{.BadField}}{{end}}`,
			Data:     NewIterators(),
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "BadField not found")
			},
		},
		{
			Name:     "range iter2 body is checked",
			Template: `{{range .Method2}}{{.BadField}}{{end}}`,
			Data:     NewIterators(),
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "BadField not found")
			},
		},
		{
			Name:     "range second variable type is correct",
			Template: `{{range $k, $v := .Numbers}}{{expectFloat64 $v}}{{end}}`,
			Data: MethodWithKeyValForSlices{
				Numbers: []float64{1, 2},
			},
		},
		{
			Name:     "range over an untyped constant",
			Template: `{{range 1.2}}{{end}}`,
			Data:     nil,
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				// The "untyped " prefix go/types uses for constant types is
				// trimmed: the template author wrote a float, not a Go
				// "untyped float".
				require.ErrorContains(t, checkErr, "range can't iterate over float")
				require.NotContains(t, checkErr.Error(), "untyped")
			},
		},
		{
			Name:     "range over int data body has bad field",
			Template: `{{range .}}{{.NoSuchField}}{{end}}`,
			Data:     int(0),
			Error: func(t *testing.T, checkErr error, _ types.Type) {
				require.ErrorContains(t, checkErr, "NoSuchField")
			},
		},
		{
			Name:     "range over int data else has bad field",
			Template: `{{range .}}body{{else}}{{.NoSuchField}}{{end}}`,
			Data:     int(0),
			Error: func(t *testing.T, checkErr error, _ types.Type) {
				require.ErrorContains(t, checkErr, "NoSuchField")
			},
		},
		{
			Name:     "range over iter.Seq body has bad field",
			Template: `{{range .Field}}{{.NoSuchField}}{{end}}`,
			Data:     NewIterators(),
			Error: func(t *testing.T, checkErr error, _ types.Type) {
				require.ErrorContains(t, checkErr, "NoSuchField")
			},
		},
		{
			Name:     "range over iter.Seq2 body has bad field",
			Template: `{{range .Field2}}{{.NoSuchField}}{{end}}`,
			Data:     NewIterators(),
			Error: func(t *testing.T, checkErr error, _ types.Type) {
				require.ErrorContains(t, checkErr, "NoSuchField")
			},
		},
		{
			Name:     "range over iter.Seq2 with two vars: dot is value type",
			Template: `{{range $k, $v := .Field2}}{{expectFloat64 .}}{{end}}`,
			Data:     NewIterators(),
		},
		{
			Name:     "template with bool argument downgrades untyped bool",
			Template: `{{define "tb"}}{{expectBool .}}{{end}}{{template "tb" true}}`,
			Data:     Void{},
		},
		{
			Name:     "template with bool argument error shows bool not untyped bool",
			Template: `{{define "tb2"}}{{expectInt8 .}}{{end}}{{template "tb2" true}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "has type bool expected int8")
			},
		},
		{
			Name:     "slice builtin returns slice type not element type",
			Template: `{{.ExpectSliceInt (slice .SI 1 2)}}`,
			Data:     StructWithSliceInt{SI: []int{1, 2, 3}},
		},
		{
			Name:     "range over channel single variable",
			Template: `{{range $v := .Chan}}{{expectInt $v}}{{end}}`,
			Data:     StructWithChan{},
		},
		{
			Name:     "and with no arguments error says and",
			Template: `{{and}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "built-in and")
			},
		},
		{
			Name:     "or with no arguments error says or",
			Template: `{{or}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "built-in or")
			},
		},
		{
			Name:     "block node with pipeline",
			Template: `{{block "myblock" .Field}}{{expectString .}}{{end}}`,
			Data:     StructWithStringField{},
		},
		{
			Name:     "block node with nil pipeline",
			Template: `{{block "nilblock" .}}hello{{end}}`,
			Data:     Void{},
		},
		{
			Name:     "block node body type error",
			Template: `{{block "badblock" .Field}}{{expectInt .}}{{end}}`,
			Data:     StructWithStringField{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "expected int")
			},
		},
		{
			Name:     "identifier node with unknown function",
			Template: `{{unknownFunc}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr error, tp types.Type) {
				require.ErrorContains(t, checkErr, "unknown function: unknownFunc")
			},
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			sourceFunctions := check.DefaultFunctions(testPkg.Types)
			for name, sig := range functionSignatures(t, testPkg, funcNames...) {
				sourceFunctions[name] = sig
			}

			tmpl := template.New[*types.Signature]("template")
			tmpl = tmpl.Functions(sourceFunctions)
			tmpl, parseErr := tmpl.Parse(tt.Template)
			require.NoError(t, parseErr)

			dataType := treeTestRowType(t, testPkg, ttRows, tt.Name)

			global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, sourceFunctions)

			if checkErr := check.Execute(global, tmpl.Tree(), dataType); tt.Error != nil {
				tt.Error(t, checkErr, dataType)
			} else {
				require.NoError(t, checkErr)
			}
		})
	}

	t.Run("field on interface", func(t *testing.T) {
		tp := testPkg.Types.Scope().Lookup("Fooer").Type()
		fooer, ok := tp.(*types.Named)
		require.True(t, ok)
		obj, _, _ := types.LookupFieldOrMethod(fooer, false, testPkg.Types, "Foo")
		require.NotNil(t, obj)

		tmpl := template.New[*types.Signature]("")
		tmpl, err := tmpl.Parse(`{{.Foo}}`)
		require.NoError(t, err)
		global := check.NewGlobal(testPkg.Types, testPkg.Fset, nil, nil)
		require.NoError(t, check.Execute(global, tmpl.Tree(), fooer))
	})
	t.Run("field on parenthesized interface", func(t *testing.T) {
		tp := testPkg.Types.Scope().Lookup("Fooer").Type()
		fooer, ok := tp.(*types.Named)
		require.True(t, ok)
		obj, _, _ := types.LookupFieldOrMethod(fooer, false, testPkg.Types, "Foo")
		require.NotNil(t, obj)

		tmpl := template.New[*types.Signature]("")
		tmpl, err := tmpl.Parse(`{{.Foo}}`)
		require.NoError(t, err)
		global := check.NewGlobal(testPkg.Types, testPkg.Fset, nil, nil)
		require.NoError(t, check.Execute(global, tmpl.Tree(), fooer))
	})
}

func TestTemplateNodeTypeHook(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})

	tmpl := template.New[*types.Signature]("main")
	tmpl, err := tmpl.Parse(`{{define "sub"}}{{.Field}}{{end}}{{template "sub" .Data}}`)
	require.NoError(t, err)

	type HookCall struct {
		treeName   string
		nodeName   string
		typeString string
	}

	testType := types.NewStruct(
		[]*types.Var{
			types.NewField(0, testPkg.Types, "Data", types.NewStruct(
				[]*types.Var{
					types.NewField(0, testPkg.Types, "Field", types.Typ[types.String], true),
				},
				nil,
			), true),
		},
		nil,
	)

	global := check.NewGlobal(
		testPkg.Types,
		testPkg.Fset,
		tmpl,
		check.DefaultFunctions(testPkg.Types),
	)

	var hookCalls []HookCall
	global.InspectTemplateNode = func(node *template.TemplateNode, t *template.Tree, tp types.Type, _ check.Definition) {
		hookCalls = append(hookCalls, HookCall{
			treeName:   t.Name,
			nodeName:   node.Name,
			typeString: tp.String(),
		})
	}

	tree, ok := tmpl.FindTree("main")
	require.True(t, ok)
	err = check.Execute(global, tree, testType)
	require.NoError(t, err)

	require.Len(t, hookCalls, 1, "hook should be called once for the template node")

	call := hookCalls[0]
	assert.Equal(t, "main", call.treeName, "tree name should be 'main'")
	assert.Equal(t, "sub", call.nodeName, "node name should be 'sub'")

	assert.Contains(t, call.typeString, "struct{", "type should be a struct")
	assert.Contains(t, call.typeString, "string", "type should contain string field")
}

func TestNodeTypeHook(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})

	tmpl := template.New[*types.Signature]("main")
	// "sub" is invoked from two call sites with different dot types: .Data
	// (whose Field is string) and .Other (whose Field is int).
	tmpl, err := tmpl.Parse(`{{.Data}}{{define "sub"}}{{.Field}}{{end}}{{template "sub" .Data}}{{template "sub" .Other}}`)
	require.NoError(t, err)

	// data is struct{ Data struct{ Field string }; Other struct{ Field int } }
	testType := types.NewStruct(
		[]*types.Var{
			types.NewField(0, testPkg.Types, "Data", types.NewStruct(
				[]*types.Var{
					types.NewField(0, testPkg.Types, "Field", types.Typ[types.String], true),
				},
				nil,
			), true),
			types.NewField(0, testPkg.Types, "Other", types.NewStruct(
				[]*types.Var{
					types.NewField(0, testPkg.Types, "Field", types.Typ[types.Int], true),
				},
				nil,
			), true),
		},
		nil,
	)

	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, check.DefaultFunctions(testPkg.Types))

	var infos []check.NodeTypeInfo
	global.InspectNode = func(info check.NodeTypeInfo) {
		infos = append(infos, info)
	}

	tree, ok := tmpl.FindTree("main")
	require.True(t, ok)
	require.NoError(t, check.Execute(global, tree, testType))

	// The .Data field access in "main" resolves to the inner struct.
	dataField := find(t, infos, func(i check.NodeTypeInfo) bool {
		_, ok := i.Node.(*template.FieldNode)
		return ok && i.Tree.Name == "main" && i.Type != nil &&
			strings.Contains(i.Type.String(), "struct{") && strings.Contains(i.Type.String(), "string")
	})
	require.NotNil(t, dataField.Type)

	// The .Field access inside "sub" is typed with the dot each {{template
	// "sub" ...}} invocation passes — proving per-execution typing flows
	// through nested template invocations, and that the SAME sub node
	// reports a different type per call path (string via .Data, int via
	// .Other). This per-call-path property is what the call-stack increment
	// builds on.
	var subFieldTypes []string
	for _, i := range infos {
		if _, ok := i.Node.(*template.FieldNode); ok && i.Tree.Name == "sub" && i.Type != nil {
			subFieldTypes = append(subFieldTypes, i.Type.String())
		}
	}
	require.ElementsMatch(t, []string{"string", "int"}, subFieldTypes,
		"sub's .Field should be typed once per call path")

	// The field node also carries the Go object it resolves to (for
	// go-to-definition), and only field nodes do.
	dataObj := find(t, infos, func(i check.NodeTypeInfo) bool {
		fn, ok := i.Node.(*template.FieldNode)
		return ok && i.Tree.Name == "main" && len(fn.Ident) > 0 && fn.Ident[len(fn.Ident)-1] == "Data"
	})
	require.NotNil(t, dataObj.Object, "field node should carry its resolved object")
	assert.Equal(t, "Data", dataObj.Object.Name())
	// (Pos validity is exercised at the LSP layer against a real loaded
	// package; the synthetic testType here has NoPos fields.)
}

func TestNodeTypeHookMapIndexHasNoObject(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})

	tmpl := template.New[*types.Signature]("main")
	tmpl, err := tmpl.Parse(`{{.M.Key}}`)
	require.NoError(t, err)

	// data is struct{ M map[string]int }
	dataType := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "M",
			types.NewMap(types.Typ[types.String], types.Typ[types.Int]), true),
	}, nil)

	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, check.DefaultFunctions(testPkg.Types))
	var infos []check.NodeTypeInfo
	global.InspectNode = func(i check.NodeTypeInfo) { infos = append(infos, i) }

	tree, ok := tmpl.FindTree("main")
	require.True(t, ok)
	require.NoError(t, check.Execute(global, tree, dataType))

	// The last segment (.Key) is a map index with no Go declaration, so the
	// node must carry no Object — otherwise go-to-definition would wrongly
	// jump to M's definition.
	fieldInfo := find(t, infos, func(i check.NodeTypeInfo) bool {
		fn, ok := i.Node.(*template.FieldNode)
		return ok && len(fn.Ident) == 2 && fn.Ident[0] == "M" && fn.Ident[1] == "Key"
	})
	assert.Equal(t, "int", fieldInfo.Type.String())
	assert.Nil(t, fieldInfo.Object, "map index segment must not carry an object")
}

func TestMembers(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	st := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "Name", types.Typ[types.String], false),
		types.NewField(0, testPkg.Types, "Age", types.Typ[types.Int], false),
		types.NewField(0, testPkg.Types, "hidden", types.Typ[types.Bool], false),
	}, nil)

	got := check.Members(st)
	var names []string
	for _, m := range got {
		names = append(names, m.Name)
	}
	assert.Equal(t, []string{"Age", "Name"}, names, "sorted, unexported excluded")
	assert.Equal(t, check.MemberField, got[0].Kind)
	assert.Equal(t, "int", got[0].Type)

	// A pointer dot (Execute with &T{}) must report the pointee's members,
	// not an empty set.
	ptrMembers := check.Members(types.NewPointer(st))
	var ptrNames []string
	for _, m := range ptrMembers {
		ptrNames = append(ptrNames, m.Name)
	}
	assert.Equal(t, []string{"Age", "Name"}, ptrNames, "pointer type is dereferenced")
}

func TestBareDotEmitsDotType(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	tmpl, err := template.New[*types.Signature]("main").Parse(`{{.}}`)
	require.NoError(t, err)
	dt := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "Name", types.Typ[types.String], false),
	}, nil)

	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, check.DefaultFunctions(testPkg.Types))
	var infos []check.NodeTypeInfo
	global.InspectNode = func(i check.NodeTypeInfo) { infos = append(infos, i) }
	tree, ok := tmpl.FindTree("main")
	require.True(t, ok)
	require.NoError(t, check.Execute(global, tree, dt))

	// {{.}} emits a DotNode carrying the dot type, so a completion probe can
	// read the dot's members for the first selector segment.
	dotInfo := find(t, infos, func(i check.NodeTypeInfo) bool {
		_, ok := i.Node.(*template.DotNode)
		return ok
	})
	require.NotNil(t, dotInfo.Type)
	assert.Contains(t, dotInfo.Type.String(), "Name")
	// The dot in effect is recorded too; for a bare dot it equals the type.
	require.NotNil(t, dotInfo.Dot)
	assert.Equal(t, dotInfo.Type.String(), dotInfo.Dot.String())
}

func TestNodeTypeHookEmitsVars(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	tmpl, err := template.New[*types.Signature]("main").Parse(
		`{{range $i, $v := .Items}}{{$v}}{{end}}`,
	)
	require.NoError(t, err)
	dt := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "Items",
			types.NewSlice(types.Typ[types.String]), false),
	}, nil)

	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, check.DefaultFunctions(testPkg.Types))
	var infos []check.NodeTypeInfo
	global.InspectNode = func(i check.NodeTypeInfo) { infos = append(infos, i) }
	tree, ok := tmpl.FindTree("main")
	require.True(t, ok)
	require.NoError(t, check.Execute(global, tree, dt))

	// The {{$v}} inside the range body carries the range's loop variables in
	// scope, so variable completion there offers $i (int) and $v (string).
	vInfo := find(t, infos, func(i check.NodeTypeInfo) bool {
		v, ok := i.Node.(*template.VariableNode)
		return ok && len(v.Ident) > 0 && v.Ident[0] == "$v"
	})
	require.NotNil(t, vInfo.Vars)
	assert.Equal(t, "string", vInfo.Vars["$v"].String())
	assert.Equal(t, "int", vInfo.Vars["$i"].String())
	assert.Contains(t, vInfo.Vars, "$", "the root variable is always in scope")
}

func TestNodeTypeVarsSnapshotIsFrozen(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	// $a is declared, then referenced, then $b is declared. The {{$a}}
	// reference must not see $b — its Vars snapshot is frozen at emit and
	// must not pick up a variable declared later in the same list.
	tmpl, err := template.New[*types.Signature]("main").Parse(
		`{{$a := .Name}}{{$a}}{{$b := .Name}}{{$b}}`,
	)
	require.NoError(t, err)
	dt := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "Name", types.Typ[types.String], false),
	}, nil)

	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, check.DefaultFunctions(testPkg.Types))
	var infos []check.NodeTypeInfo
	global.InspectNode = func(i check.NodeTypeInfo) { infos = append(infos, i) }
	tree, ok := tmpl.FindTree("main")
	require.True(t, ok)
	require.NoError(t, check.Execute(global, tree, dt))

	varRef := func(name string) check.NodeTypeInfo {
		return find(t, infos, func(i check.NodeTypeInfo) bool {
			v, ok := i.Node.(*template.VariableNode)
			return ok && len(v.Ident) > 0 && v.Ident[0] == name
		})
	}
	// {{$a}} sees $ and $a but not the later $b.
	a := varRef("$a")
	assert.Contains(t, a.Vars, "$a")
	assert.NotContains(t, a.Vars, "$b", "a variable declared later must not leak in")
	// {{$b}} sees both, confirming forward accumulation still works.
	assert.Contains(t, varRef("$b").Vars, "$b")
}

func TestErrorPositionAndUnwrap(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	tmpl, err := template.New[*types.Signature]("main").Parse("x{{.Missing}}y")
	require.NoError(t, err)
	dt := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "Name", types.Typ[types.String], false),
	}, nil)

	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, check.DefaultFunctions(testPkg.Types))
	tree, ok := tmpl.FindTree("main")
	require.True(t, ok)
	tree.Filepath = "page.gohtml"
	execErr := check.Execute(global, tree, dt)
	require.Error(t, execErr)

	var ce *check.Error
	require.ErrorAs(t, execErr, &ce)

	// Position/EndPosition bracket the offending node by byte offset, so a
	// diagnostic can be placed without parsing the Error() string.
	const src = "x{{.Missing}}y"
	start := ce.Position()
	end := ce.EndPosition()
	assert.Equal(t, "page.gohtml", start.Filename)
	assert.Greater(t, end.Offset, start.Offset)
	assert.Equal(t, ".Missing", src[start.Offset:end.Offset], "span covers the field node")

	// The unwrapped error is the bare, position-free message — no
	// "file:line:col: executing ... at <...>:" prefix.
	require.NotNil(t, ce.Unwrap())
	msg := ce.Unwrap().Error()
	assert.NotContains(t, msg, "executing")
	assert.NotContains(t, msg, "page.gohtml")
	assert.Contains(t, ce.Error(), msg, "the full Error keeps the bare message as its tail")
}

func find[T any](t *testing.T, list []T, match func(p T) bool) T {
	t.Helper()
	if i := slices.IndexFunc(list, match); i >= 0 {
		return list[i]
	} else {
		var zero T
		t.Fatalf("failed to find")
		return zero
	}
}

func treeTestRowType(t *testing.T, p *packages.Package, ttRows *ast.CompositeLit, name string) types.Type {
	t.Helper()
	require.NotNil(t, ttRows)
	rowNames := parseRowNames(t, p, ttRows)
	i := rowNames[name]
	return getRowType(t, p, ttRows.Elts[i].(*ast.CompositeLit))
}

func parseRowNames(t *testing.T, p *packages.Package, ttRows *ast.CompositeLit) map[string]int {
	rowNames := make(map[string]int)
	for i, r := range ttRows.Elts {
		row, ok := r.(*ast.CompositeLit)
		if !ok {
			continue
		}
		if len(row.Elts) < 1 {
			continue
		}
		for _, elt := range row.Elts {
			pair, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				t.Fatalf("expected key/value pair at%s", p.Fset.Position(elt.Pos()))
			}
			key, ok := pair.Key.(*ast.Ident)
			if !ok {
				t.Fatalf("expected ident key at test %s", p.Fset.Position(elt.Pos()))
			}
			if key.Name != "Name" {
				continue
			}
			value, ok := pair.Value.(*ast.BasicLit)
			if !ok {
				t.Fatalf("expected basic lit at test %s", p.Fset.Position(elt.Pos()))
			}
			name, err := strconv.Unquote(value.Value)
			require.NoError(t, err)
			rowNames[name] = i
		}
	}
	return rowNames
}

func getRowType(t *testing.T, p *packages.Package, row *ast.CompositeLit) types.Type {
	t.Helper()
	for i, elt := range row.Elts {
		pair, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			pos := p.Fset.Position(elt.Pos())
			t.Fatalf("expected key/value pair at %s", pos)
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok {
			t.Fatalf("expected ident key at test %d", i)
		}
		if key.Name != "Data" {
			continue
		}
		var buf bytes.Buffer
		require.NoError(t, format.Node(&buf, p.Fset, pair.Value))
		result, err := types.Eval(p.Fset, p.Types, pair.Value.Pos(), buf.String())
		require.NoError(t, err)
		tp := result.Type
		require.NotNil(t, tp)
		return tp
	}
	t.Fatalf("failed to evaluate type for row")
	return nil
}

func TestErrorUnwrap(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})

	sourceFunctions := check.DefaultFunctions(testPkg.Types)

	tmpl := template.New[*types.Signature]("template")
	tmpl = tmpl.Functions(sourceFunctions)
	tmpl, err := tmpl.Parse(`{{.MissingField}}`)
	require.NoError(t, err)

	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, sourceFunctions)
	checkErr := check.Execute(global, tmpl.Tree(), types.Typ[types.Int])

	require.Error(t, checkErr)

	var checkError *check.Error
	require.True(t, errors.As(checkErr, &checkError), "error should be a *check.Error")

	unwrapped := errors.Unwrap(checkError)
	require.NotNil(t, unwrapped, "Unwrap should return the inner error")
	assert.Contains(t, unwrapped.Error(), "MissingField not found")
}

func TestFindTreeFunc(t *testing.T) {
	tree1 := &template.Tree{Name: "hello"}
	tree2 := &template.Tree{Name: "world"}

	lookup := check.FindTreeFunc(func(name string) (*template.Tree, bool) {
		switch name {
		case "hello":
			return tree1, true
		case "world":
			return tree2, true
		default:
			return nil, false
		}
	})

	t.Run("found", func(t *testing.T) {
		tree, ok := lookup.FindTree("hello")
		require.True(t, ok)
		assert.Equal(t, "hello", tree.Name)
	})

	t.Run("found second", func(t *testing.T) {
		tree, ok := lookup.FindTree("world")
		require.True(t, ok)
		assert.Equal(t, "world", tree.Name)
	})

	t.Run("not found", func(t *testing.T) {
		tree, ok := lookup.FindTree("missing")
		assert.False(t, ok)
		assert.Nil(t, tree)
	})
}

func TestFunctionsAdd(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})

	base := check.DefaultFunctions(testPkg.Types)
	origLen := len(base)

	extra := functionSignatures(t, testPkg, "square")
	extraFunctions := make(check.Functions)
	for name, sig := range extra {
		extraFunctions[name] = sig
	}

	merged := base.Add(extraFunctions)

	// merged should have the extra functions
	assert.Greater(t, len(merged), origLen)

	// original should be unchanged
	assert.Equal(t, origLen, len(base), "Add should not mutate the receiver")

	// verify the added function is present and callable with correct arg type
	_, err := merged.CheckCall(
		check.NewGlobal(testPkg.Types, testPkg.Fset, nil, nil),
		"square", nil, []types.Type{types.Typ[types.Int]},
	)
	require.NoError(t, err, "square should be callable after Add")

	// verify original built-in still works
	_, err = merged.CheckCall(
		check.NewGlobal(testPkg.Types, testPkg.Fset, nil, nil),
		"len", nil, []types.Type{types.NewSlice(types.Typ[types.Int])},
	)
	require.NoError(t, err, "len should still work after Add")
}

func TestIdentifierNodeBuiltins(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})

	sourceFunctions := check.DefaultFunctions(testPkg.Types)
	for name, sig := range functionSignatures(t, testPkg, "square", "expectInt") {
		sourceFunctions[name] = sig
	}

	lookupType := func(name string) types.Type {
		t.Helper()
		obj := testPkg.Types.Scope().Lookup(name)
		require.NotNil(t, obj, "type %q not found in package scope", name)
		return obj.Type()
	}

	tests := []struct {
		name     string
		template string
		dataType types.Type
		errMsg   string
	}{
		{
			name:     "built-in len on slice",
			template: `{{len .SI}}`,
			dataType: types.NewPointer(lookupType("T")),
		},
		{
			name:     "custom function from funcmap",
			template: `{{square 5}}`,
			dataType: lookupType("Void"),
		},
		{
			name:     "undefined identifier",
			template: `{{completelyUnknown}}`,
			dataType: lookupType("Void"),
			errMsg:   "unknown function: completelyUnknown",
		},
		{
			name:     "print built-in",
			template: `{{print "hello"}}`,
			dataType: lookupType("Void"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := template.New[*types.Signature]("template")
			tmpl = tmpl.Functions(sourceFunctions)
			tmpl, parseErr := tmpl.Parse(tt.template)
			require.NoError(t, parseErr)

			global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, sourceFunctions)

			err := check.Execute(global, tmpl.Tree(), tt.dataType)
			if tt.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func detailedError(t *testing.T, err error) string {
	t.Helper()
	var checkErr *check.Error
	require.ErrorAs(t, err, &checkErr)
	var sb strings.Builder
	require.NoError(t, checkErr.DetailedError(&sb, nil))
	return sb.String()
}
