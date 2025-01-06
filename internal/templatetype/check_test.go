package templatetype_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/types"
	"html/template"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/crhntr/muxt/internal/templatetype"
)

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

func TestTree(t *testing.T) {
	const testFuncName = "TestTree"
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == "templatetype_test"
	})

	fileIndex := slices.IndexFunc(testPkg.Syntax, func(file *ast.File) bool {
		pos := testPkg.Fset.Position(file.Pos())
		return file.Name.Name == "templatetype_test" && filepath.Base(pos.Filename) == "check_test.go"
	})
	if fileIndex < 0 {
		t.Fatal("no check_test.go found")
	}
	file := testPkg.Syntax[fileIndex]

	type ttRow struct {
		Name     string
		Template string
		Data     any
		Error    func(t *testing.T, checkErr, execErr error, tp types.Type)
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

	for _, tt := range []ttRow{
		{
			Name:     "when accessing nil on an empty struct",
			Template: `{{.Field}}`,
			Data:     Void{},
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				require.EqualError(t, err, fmt.Sprintf(`type check failed: template:1:2: Field not found on %s`, tp))
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
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				method, _, _ := types.LookupFieldOrMethod(tp, true, testPkg.Types, "Method")
				require.NotNil(t, method)
				methodPos := testPkg.Fset.Position(method.Pos())

				require.EqualError(t, err, fmt.Sprintf(`type check failed: template:1:2: function Method has 0 return values; should be 1 or 2: incorrect signature at %s`, methodPos))
			},
		},
		{
			Name:     "when a method does has a result",
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
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				method, _, _ := types.LookupFieldOrMethod(tp, true, testPkg.Types, "Method")
				require.NotNil(t, method)
				methodPos := testPkg.Fset.Position(method.Pos())

				require.EqualError(t, err, fmt.Sprintf(`type check failed: template:1:2: invalid function signature for Method: second return value should be error; is int: incorrect signature at %s`, methodPos))
			},
		},
		{
			Name:     "when a method with too many results",
			Template: `{{.Method}}`,
			Data:     TypeWithMethodSignatureThreeResults{},
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				method, _, _ := types.LookupFieldOrMethod(tp, true, testPkg.Types, "Method")
				require.NotNil(t, method)
				methodPos := testPkg.Fset.Position(method.Pos())

				require.EqualError(t, err, fmt.Sprintf(`type check failed: template:1:2: function Method has 3 return values; should be 1 or 2: incorrect signature at %s`, methodPos))
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
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				m1, _, _ := types.LookupFieldOrMethod(tp, true, testPkg.Types, "Method")
				require.NotNil(t, m1)
				m2, _, _ := types.LookupFieldOrMethod(m1.Type().(*types.Signature).Results().At(0).Type(), true, testPkg.Types, "Method")
				require.NotNil(t, m2)
				methodPos := testPkg.Fset.Position(m2.Pos())

				require.EqualError(t, err, fmt.Sprintf(`type check failed: template:1:9: function Method has 0 return values; should be 1 or 2: incorrect signature at %s`, methodPos))
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
			Name:     "when the struct has the field and the field has a method",
			Template: `{{.Field}}`,
			Data:     StructWithFieldWithMethod{},
		},
		{
			Name:     "when the struct has the field of kind func",
			Template: `{{.Func.Method}}`,
			Data: StructWithFuncFieldWithResultWithMethod{
				Func: func() (_ TypeWithMethodSignatureResult) { return },
			},
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				fn, _, _ := types.LookupFieldOrMethod(tp, true, testPkg.Types, "Func")
				require.NotNil(t, fn)
				require.ErrorContains(t, err, fmt.Sprintf(`type check failed: template:1:7: executing "template" at <.Func.Method>: identifier chain not supported for type %s`, fn.Type()))
			},
		},
		{
			Name:     "when a method has an int parameter",
			Template: `{{.F 21}}`,
			Data:     MethodWithIntParam{},
		},
		{
			Name:     "when a method argument is an bool but param is int",
			Template: `{{.F false}}`,
			Data:     MethodWithIntParam{},
			Error: func(t *testing.T, checkErr, _ error, tp types.Type) {
				require.Error(t, checkErr)
				require.ErrorContains(t, checkErr, "expected int")
			},
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
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.Error(t, checkErr)
				require.ErrorContains(t, checkErr, "expected bool")
				require.Error(t, execErr)
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
			Name:     "when a variable is used",
			Template: `{{$v := 1}}{{.F $v}}`,
			Data:     MethodWithIntParam{},
		},
		{
			Name:     "when there is an error in the else block",
			Template: `{{$x := "wrong type"}}{{if false}}{{else}}{{.F $x}}{{end}}`,
			Data:     MethodWithIntParam{},
			Error: func(t *testing.T, checkErr, _ error, tp types.Type) {
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
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.ErrorContains(t, execErr, "wrong type for value; expected int; got string")
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
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.NoError(t, execErr)
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
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.NoError(t, execErr)
				require.ErrorContains(t, checkErr, "argument 0 has type float64 expected int")
			},
		},
		{
			Name:     "it downgrades untyped integers",
			Template: `{{define "t"}}{{expectInt8 .}}{{end}}{{if false}}{{template "t" 12}}{{end}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.NoError(t, execErr)
				require.ErrorContains(t, checkErr, "argument 0 has type int expected int8")
			},
		},
		//{
		//	Name:     "it downgrades untyped floats",
		//	Template: `{{define "t"}}{{expectFloat32 .}}{{end}}{{if false}}{{template "t" 1.2}}{{end}}`,
		//	Data:     Void{},
		//	Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
		//		require.EqualError(t, checkErr, convertTextExecError(t, execErr))
		//	},
		//},
		//{
		//	Name:     "it downgrades untyped complex",
		//	Template: `{{define "t"}}{{expectComplex64 .}}{{end}}{{if false}}{{template "t" 2i}}{{end}}`,
		//	Data:     Void{},
		//	Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
		//		require.EqualError(t, checkErr, convertTextExecError(t, execErr))
		//	},
		//},
		// not sure if I should be downgrading bool, it should be fine to let it be since there is only one basic bool type
		{
			Name:     "chain node",
			Template: `{{(.).A.B.C.D}}`,
			Data:     LetterChainA{},
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.NoError(t, execErr)
				require.NoError(t, checkErr)
			},
		},
		{
			Name:     "chain node with type change in term",
			Template: `{{(.A).B.C.D}}`,
			Data:     LetterChainA{},
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.NoError(t, execErr)
				require.NoError(t, checkErr)
			},
		},

		// stdlib exec tests

		// Trivial cases.
		// {"empty", "", "", nil, true},
		{
			Name:     "empty",
			Template: "",
			Data:     Void{},
		},
		// {"text", "some text", "some text", nil, true},
		{
			Name:     "text",
			Template: "some text",
			Data:     Void{},
		},
		// {"nil action", "{{nil}}", "", nil, false},
		{
			Name:     "nil action",
			Template: `{{nil}}`,
			Data:     Void{},
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.EqualError(t, checkErr, convertTextExecError(t, execErr))
			},
		},

		// Ideal constants.
		// {"ideal int", "{{typeOf 3}}", "int", 0, true},
		{
			Name:     "ideal int",
			Template: `{{expectInt 3}}`,
			Data:     Void{},
		},
		// {"ideal float", "{{typeOf 1.0}}", "float64", 0, true},
		{
			Name:     "ideal float",
			Template: `{{expectFloat64 1.0}}}`,
			Data:     Void{},
		},
		// {"ideal exp float", "{{typeOf 1e1}}", "float64", 0, true},
		{
			Name:     "ideal exponent",
			Template: `{{expectFloat64 1e1}}`,
			Data:     Void{},
		},
		// {"ideal complex", "{{typeOf 1i}}", "complex128", 0, true},
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
		// {".U.V", "-{{.U.V}}-", "-v-", tVal, true},
		{
			Name:     ".U.V",
			Template: "-{{.U.V}}-",
			Data:     tVal,
		},
		// {".unexported", "{{.unexported}}", "", tVal, false},
		{
			Name:     ".unexported", // copied from stdlib
			Template: "{{.unexported}}",
			Data:     tVal,
			Error: func(t *testing.T, checkErr, execErr error, _ types.Type) {
				require.Error(t, checkErr)
				require.Error(t, execErr)
			},
		},
		{
			Name:     "Interface call", // copied from stdlib
			Template: `{{stringer .S}}`,
			Data: map[string]fmt.Stringer{
				"S": bytes.NewBufferString("foozle"),
			},
		},
		{
			Name:     "error method, error", // copied from stdlib
			Template: "{{.MyError true}}",
			Data:     tVal,
			Error: func(t *testing.T, checkErr, execErr error, _ types.Type) {
				require.NoError(t, checkErr)
				require.Error(t, execErr)
			},
		},
		{
			Name:     "nil call arg", // copied from stdlib
			Template: `{{ call .TVal.NilOKFunc .NilInt }}`,
			Data: &struct {
				TVal   *T
				NilInt *int
			}{
				TVal: tVal,
			},
		},
		{
			Name:     "len of an interface field", // copied from stdlib
			Template: "{{len .Empty3}}",
			Data:     tVal,
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				require.NoError(t, execErr)
				require.ErrorContains(t, checkErr, "built-in len expects the first argument to be an array, slice, map, or string got any")
			},
		},
		{
			Name:     "and undef", // copied from stdlib
			Template: "{{and 1 .Unknown}}",
			Data:     nil,
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				assert.NoError(t, execErr)
				require.ErrorContains(t, checkErr, "type check failed: template:1:8: Unknown not found on untyped nil")
			},
		},
		{
			Name:     "or undef", // copied from stdlib
			Template: "{{or 0 .Unknown}}",
			Data:     nil,
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				assert.NoError(t, execErr)
				require.ErrorContains(t, checkErr, "type check failed: template:1:7: Unknown not found on untyped nil")
			},
		},
		{
			Name:     "slice[HUGE]",
			Template: "{{index . 10}}",
			Data:     [3]int{},
			Error: func(t *testing.T, checkErr, execErr error, tp types.Type) {
				t.Skip("need to figure out how to pass type and value back")
				require.Error(t, execErr)
				require.ErrorContains(t, checkErr, "out of range")
			},
		},
		{
			Name:     "nil pipeline", // copied from stdlib
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
	} {
		t.Run(tt.Name, func(t *testing.T) {
			functions := template.FuncMap{
				"square":           square,
				"ceil":             ceil,
				"expectInt":        expectInt,
				"expectFloat64":    expectFloat64,
				"expectString":     expectString,
				"expectInt8":       expectInt8,
				"expectFloat32":    expectFloat32,
				"expectComplex64":  expectComplex64,
				"expectComplex128": expectComplex128,
				"typeOf":           typeOf,
				"stringer":         stringer,
				"echoT":            echoT,
			}

			templates, parseErr := template.New("template").Funcs(functions).Parse(tt.Template)
			require.NoError(t, parseErr)

			dataType := treeTestRowType(t, testPkg, ttRows, tt.Name)

			sourceFunctions := templatetype.DefaultFunctions(testPkg.Types)
			for name := range functions {
				fn := testPkg.Types.Scope().Lookup(name).(*types.Func).Signature()
				require.NotNil(t, fn)
				sourceFunctions[name] = fn
			}

			if checkErr := templatetype.Check(templates.Tree, dataType, testPkg.Types, testPkg.Fset, templatetype.FindTreeFunc(func(name string) (*parse.Tree, bool) {
				ts := templates.Lookup(name)
				if ts == nil {
					return nil, false
				}
				return ts.Tree, true
			}), sourceFunctions); tt.Error != nil {
				execErr := templates.Execute(io.Discard, tt.Data)
				tt.Error(t, checkErr, execErr, dataType)
			} else {
				execErr := templates.Execute(io.Discard, tt.Data)
				require.NoError(t, checkErr)
				require.NoError(t, execErr)
			}
		})
	}

	t.Run("field on interface", func(t *testing.T) {
		tp := testPkg.Types.Scope().Lookup("Fooer").Type()
		fooer, ok := tp.(*types.Named)
		require.True(t, ok)
		obj, _, _ := types.LookupFieldOrMethod(fooer, false, testPkg.Types, "Foo")
		require.NotNil(t, obj)

		templ := template.Must(template.New("").Parse(`{{.Foo}}`))
		require.NoError(t, templatetype.Check(templ.Tree, fooer, testPkg.Types, testPkg.Fset, nil, nil))

	})
	t.Run("field on parenthesized interface", func(t *testing.T) {
		tp := testPkg.Types.Scope().Lookup("Fooer").Type()
		fooer, ok := tp.(*types.Named)
		require.True(t, ok)
		obj, _, _ := types.LookupFieldOrMethod(fooer, false, testPkg.Types, "Foo")
		require.NotNil(t, obj)

		templ := template.Must(template.New("").Parse(`{{.Foo}}`))
		require.NoError(t, templatetype.Check(templ.Tree, fooer, testPkg.Types, testPkg.Fset, nil, nil))
	})
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

func convertTextExecError(t *testing.T, err error) string {
	require.Error(t, err)
	return "type check failed:" + strings.TrimPrefix(err.Error(), "template:")
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
