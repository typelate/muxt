package check_test

import (
	"fmt"
	"go/types"
	"html/template"
	"io"
	"reflect"
	"slices"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/crhntr/muxt"
	"github.com/crhntr/muxt/internal/check"
	"github.com/crhntr/muxt/internal/source"
)

func TestTree(t *testing.T) {
	packageList, loadErr := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedDeps | packages.NeedTypes,
		Tests: true,
	}, ".")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	checkTestPackage := find(t, packageList, func(p *packages.Package) bool {
		return p.Name == "check_test"
	})
	for _, tt := range []struct {
		Name     string
		Template string
		Data     any
		Error    func(t *testing.T, checkErr, execErr error, tp types.Type)
	}{
		{
			Name:     "on an empty template",
			Template: ``,
			Data:     T{},
		},
		{
			Name:     "when accessing nil on an empty struct",
			Template: `{{.Field}}`,
			Data:     T{},
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				require.EqualError(t, err, fmt.Sprintf(`type check failed: template:1:2: Field not found on %s`, tp))
			},
		},
		{
			Name:     "when accessing the dot",
			Template: `{{.}}`,
			Data:     T{},
		},
		{
			Name:     "when a method does not any results",
			Template: `{{.Method}}`,
			Data:     TypeWithMethodSignatureNoResultMethod{},
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				method, _, _ := types.LookupFieldOrMethod(tp, true, checkTestPackage.Types, "Method")
				require.NotNil(t, method)
				methodPos := checkTestPackage.Fset.Position(method.Pos())

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
				method, _, _ := types.LookupFieldOrMethod(tp, true, checkTestPackage.Types, "Method")
				require.NotNil(t, method)
				methodPos := checkTestPackage.Fset.Position(method.Pos())

				require.EqualError(t, err, fmt.Sprintf(`type check failed: template:1:2: invalid function signature for Method: second return value should be error; is int: incorrect signature at %s`, methodPos))
			},
		},
		{
			Name:     "when a method with too many results",
			Template: `{{.Method}}`,
			Data:     TypeWithMethodSignatureThreeResults{},
			Error: func(t *testing.T, err, _ error, tp types.Type) {
				method, _, _ := types.LookupFieldOrMethod(tp, true, checkTestPackage.Types, "Method")
				require.NotNil(t, method)
				methodPos := checkTestPackage.Fset.Position(method.Pos())

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
				m1, _, _ := types.LookupFieldOrMethod(tp, true, checkTestPackage.Types, "Method")
				require.NotNil(t, m1)
				m2, _, _ := types.LookupFieldOrMethod(m1.Type().(*types.Signature).Results().At(0).Type(), true, checkTestPackage.Types, "Method")
				require.NotNil(t, m2)
				methodPos := checkTestPackage.Fset.Position(m2.Pos())

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
				fn, _, _ := types.LookupFieldOrMethod(tp, true, checkTestPackage.Types, "Func")
				require.NotNil(t, fn)
				require.ErrorContains(t, err, fmt.Sprintf("type check failed: template:1:7: can't evaluate field Func in type %s", fn.Type()))
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
				require.ErrorContains(t, checkErr, ".F argument 0 has type untyped string expected int")
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
			Data:     T{},
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			templates, parseErr := template.New("template").Funcs(template.FuncMap{
				"square": square,
			}).Parse(tt.Template)
			require.NoError(t, parseErr)

			dataType := checkTestPackage.Types.Scope().Lookup(reflect.TypeOf(tt.Data).Name()).Type()

			functions := source.DefaultFunctions(checkTestPackage.Types)
			functions["square"] = checkTestPackage.Types.Scope().Lookup("square").(*types.Func).Signature()

			if checkErr := check.Tree(templates.Tree, dataType, checkTestPackage.Types, checkTestPackage.Fset, newForrest(templates), functions); tt.Error != nil {
				execErr := templates.Execute(io.Discard, tt.Data)
				tt.Error(t, checkErr, execErr, dataType)
			} else {
				execErr := templates.Execute(io.Discard, tt.Data)
				require.NoError(t, execErr)
				require.NoError(t, checkErr)
			}
		})
	}
}

type T struct{}

type TypeWithMethodSignatureNoResultMethod struct{}

func (TypeWithMethodSignatureNoResultMethod) Method() {}

type TypeWithMethodSignatureResult struct{}

func (TypeWithMethodSignatureResult) Method() struct{} { return struct{}{} }

type TypeWithMethodSignatureResultAndError struct{}

func (TypeWithMethodSignatureResultAndError) Method() (struct{}, error) { return struct{}{}, nil }

type TypeWithMethodSignatureResultAndNonError struct{}

func (TypeWithMethodSignatureResultAndNonError) Method() (struct{}, int) { return struct{}{}, 0 }

type TypeWithMethodSignatureThreeResults struct{}

func (TypeWithMethodSignatureThreeResults) Method() (struct{}, struct{}, error) {
	return struct{}{}, struct{}{}, nil
}

type TypeWithMethodSignatureResultHasMethod struct{}

func (TypeWithMethodSignatureResultHasMethod) Method() (_ TypeWithMethodSignatureResult) {
	return
}

type TypeWithMethodSignatureResultHasMethodWithNoResults struct{}

func (TypeWithMethodSignatureResultHasMethodWithNoResults) Method() (_ TypeWithMethodSignatureNoResultMethod) {
	return
}

type StructWithField struct {
	Field struct{}
}

type StructWithFieldWithMethod struct {
	Field TypeWithMethodSignatureResultAndError
}

type StructWithFuncFieldWithResultWithMethod struct {
	Func func() TypeWithMethodSignatureResult
}

type MethodWithIntParam struct{}

func (MethodWithIntParam) F(int) (_ T) { return }

type MethodWithInt8Param struct{}

func (MethodWithInt8Param) F(int8) (_ T) { return }

type MethodWithInt16Param struct{}

func (MethodWithInt16Param) F(int16) (_ T) { return }

type MethodWithInt32Param struct{}

func (MethodWithInt32Param) F(int32) (_ T) { return }

type MethodWithInt64Param struct{}

func (MethodWithInt64Param) F(int64) (_ T) { return }

type MethodWithUintParam struct{}

func (MethodWithUintParam) F(uint) (_ T) { return }

type MethodWithUint8Param struct{}

func (MethodWithUint8Param) F(uint8) (_ T) { return }

type MethodWithUint16Param struct{}

func (MethodWithUint16Param) F(uint16) (_ T) { return }

type MethodWithUint32Param struct{}

func (MethodWithUint32Param) F(uint32) (_ T) { return }

type MethodWithUint64Param struct{}

func (MethodWithUint64Param) F(uint64) (_ T) { return }

type MethodWithBoolParam struct{}

func (MethodWithBoolParam) F(bool) (_ T) { return }

type MethodWithFloat64Param struct{}

func (MethodWithFloat64Param) F(float64) (_ T) { return }

type MethodWithFloat32Param struct{}

func (MethodWithFloat32Param) F(float32) (_ T) { return }

type TypeWithMethodSignatureResultMethodWithFloat32Param struct{}

func (TypeWithMethodSignatureResultMethodWithFloat32Param) Method() (_ MethodWithFloat32Param) {
	return
}

type TypeWithMethodAndSliceFloat64 struct {
	MethodWithFloat64Param
	Numbers []float64
}

type TypeWithMethodAndArrayFloat64 struct {
	MethodWithFloat64Param
	Numbers [2]float64
}

type MethodWithKeyValForSlices struct {
	Numbers []float64
}

func (MethodWithKeyValForSlices) F(int, float64) (_ T) { return }

type MethodWithKeyValForArray struct {
	Numbers [2]float64
}

func (MethodWithKeyValForArray) F(int, float64) (_ T) { return }

type MethodWithKeyValForMap struct {
	Numbers map[int16]float32
}

func (MethodWithKeyValForMap) F(int16, float32) (_ T) { return }

func square(n int) int {
	return n * n
}

func TestExampleTemplate(t *testing.T) {
	packageList, loadErr := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedDeps | packages.NeedTypes,
		Tests: true,
	}, "../../example", "net/http")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	pkg := find(t, packageList, func(p *packages.Package) bool {
		return p.PkgPath == "github.com/crhntr/muxt/example"
	})
	netHTTP := find(t, packageList, func(p *packages.Package) bool {
		return p.PkgPath == "net/http"
	})
	backend := pkg.Types.Scope().Lookup("Backend")
	require.NotNil(t, backend)

	templates, parseErr := template.ParseFiles("../../example/index.gohtml")
	require.NoError(t, parseErr)

	ts, err := muxt.Templates(templates)
	require.NoError(t, err)
	for _, mt := range ts {
		var dot types.Type
		if m := mt.Method(); m == "" {
			dot = types.NewPointer(netHTTP.Types.Scope().Lookup("Request").Type())
		} else {
			method, _, _ := types.LookupFieldOrMethod(backend.Type(), true, pkg.Types, m)
			require.NotNil(t, method)
			fn, ok := method.(*types.Func)
			require.True(t, ok)
			dot = fn.Signature().Results().At(0).Type()
		}
		require.NoError(t, check.Tree(mt.Template().Tree, dot, pkg.Types, pkg.Fset, newForrest(templates), nil))
	}
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

type Forrest template.Template

func newForrest(templates *template.Template) *Forrest {
	return (*Forrest)(templates)
}

func (forrest *Forrest) FindTree(name string) (*parse.Tree, bool) {
	ts := (*template.Template)(forrest).Lookup(name)
	if ts == nil {
		return nil, false
	}
	return ts.Tree, true
}
