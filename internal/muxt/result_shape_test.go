package muxt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkSource(t *testing.T, src string) *types.Package {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.SkipObjectResolution)
	require.NoError(t, err)
	pkg, err := new(types.Config).Check("example.com/p", fset, []*ast.File{file}, nil)
	require.NoError(t, err)
	return pkg
}

// assertErrorMessage asserts err is nil when wantErr is empty and carries
// exactly the message wantErr otherwise.
func assertErrorMessage(t *testing.T, wantErr string, err error, msgAndArgs ...any) {
	t.Helper()
	if wantErr == "" {
		assert.NoError(t, err, msgAndArgs...)
		return
	}
	assert.EqualError(t, err, wantErr, msgAndArgs...)
}

func funcSignature(t *testing.T, pkg *types.Package, name string) *types.Signature {
	t.Helper()
	obj := pkg.Scope().Lookup(name)
	require.NotNil(t, obj, "package declares no %s", name)
	return obj.Type().(*types.Signature)
}

const resultShapeSource = `package p

type T struct{}
type E struct{}

func (E) Error() string { return "" }

func None()
func One() T
func OneError() error
func OneErrorImpl() E
func TwoError() (T, error)
func TwoErrorImpl() (T, E)
func TwoBool() (T, bool)
func TwoInt() (T, int)
func TwoNamedBool() (T, namedBool)
func ErrorFirst() (error, T)
func ErrorBool() (error, bool)
func Three() (T, T, error)

type namedBool bool
`

func TestClassifyResultShape(t *testing.T) {
	pkg := checkSource(t, resultShapeSource)
	executeArgument := []Argument{{Identifier: TemplateNameScopeIdentifierExecute, Type: ArgumentTypeExecute}}
	sseExecuteArgument := []Argument{{Identifier: "other", Type: ArgumentTypeExecute}}
	for _, tt := range []struct {
		name           string
		fn             string
		representation Representation
		arguments      []Argument
		want           ResultShape
		wantErr        string
	}{
		{name: "html data", fn: "One", want: ResultShapeData},
		{name: "html data error", fn: "TwoError", want: ResultShapeDataError},
		{name: "html data error implementation", fn: "TwoErrorImpl", want: ResultShapeDataError},
		{name: "html data ok", fn: "TwoBool", want: ResultShapeDataOK},
		{name: "html second result is int", fn: "TwoInt", want: ResultShapeInvalid, wantErr: "the second result of TwoInt() (T, int) must be an error or a bool, got int"},
		{name: "html second result is a named bool", fn: "TwoNamedBool", want: ResultShapeInvalid, wantErr: "the second result of TwoNamedBool() (T, namedBool) must be an error or a bool, got namedBool"},
		{name: "html no results", fn: "None", want: ResultShapeInvalid, wantErr: "method None() has no results; it should have one or two"},
		{name: "html three results", fn: "Three", want: ResultShapeInvalid, wantErr: "method Three() (T, T, error) has 3 results; it should have one or two"},
		{name: "execute callback error", fn: "OneError", arguments: executeArgument, want: ResultShapeError},
		{name: "execute callback implementation of error", fn: "OneErrorImpl", arguments: executeArgument, want: ResultShapeError},
		{name: "execute callback data", fn: "One", arguments: executeArgument, want: ResultShapeInvalid, wantErr: "method One() T receiving the execute callback must return only error"},
		{name: "execute callback two results", fn: "TwoError", arguments: executeArgument, want: ResultShapeInvalid, wantErr: "method TwoError() (T, error) receiving the execute callback must return only error"},
		{name: "non-base execute argument leaves html rules", fn: "One", arguments: sseExecuteArgument, want: ResultShapeData},
		{name: "sse none", fn: "None", representation: RepresentationSSE, want: ResultShapeNone},
		{name: "sse error", fn: "OneError", representation: RepresentationSSE, want: ResultShapeError},
		{name: "sse data", fn: "One", representation: RepresentationSSE, want: ResultShapeInvalid, wantErr: "sse handler method One() T must return nothing or a single error"},
		{name: "sse two results", fn: "TwoError", representation: RepresentationSSE, want: ResultShapeInvalid, wantErr: "sse handler method TwoError() (T, error) must return nothing or a single error"},
		{name: "marshalJSON none", fn: "None", representation: RepresentationMarshalJSON, want: ResultShapeInvalid, wantErr: "marshalJSON requires a result to marshal but None() returns nothing"},
		{name: "marshalJSON data", fn: "One", representation: RepresentationMarshalJSON, want: ResultShapeData},
		{name: "marshalJSON only error", fn: "OneError", representation: RepresentationMarshalJSON, want: ResultShapeInvalid, wantErr: "marshalJSON requires a non-error result but OneError() error only returns an error"},
		{name: "marshalJSON data error", fn: "TwoError", representation: RepresentationMarshalJSON, want: ResultShapeDataError},
		{name: "marshalJSON error first", fn: "ErrorFirst", representation: RepresentationMarshalJSON, want: ResultShapeInvalid, wantErr: "marshalJSON requires a non-error first result to marshal but ErrorFirst() (error, T) returns an error value"},
		{name: "marshalJSON second not error", fn: "TwoBool", representation: RepresentationMarshalJSON, want: ResultShapeInvalid, wantErr: "marshalJSON requires the second result of TwoBool() (T, bool) to be an error, got bool"},
		{name: "marshalJSON three results", fn: "Three", representation: RepresentationMarshalJSON, want: ResultShapeInvalid, wantErr: "marshalJSON allows at most two results but Three() (T, T, error) has 3"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def := &Definition{
				fun:            ast.NewIdent(tt.fn),
				sig:            funcSignature(t, pkg, tt.fn),
				Representation: tt.representation,
				Arguments:      tt.arguments,
			}
			got, err := classifyResultShape(def, typeQualifier(pkg))
			assert.Equal(t, tt.want, got, "classifyResultShape(%s)", tt.fn)
			assertErrorMessage(t, tt.wantErr, err, "classifyResultShape(%s)", tt.fn)
		})
	}
}

func TestClassifyNestedCallResultShape(t *testing.T) {
	pkg := checkSource(t, resultShapeSource)
	for _, tt := range []struct {
		fn      string
		want    ResultShape
		wantErr string
	}{
		{fn: "One", want: ResultShapeData},
		{fn: "TwoError", want: ResultShapeDataError},
		{fn: "TwoBool", want: ResultShapeDataOK},
		{fn: "TwoInt", want: ResultShapeInvalid, wantErr: "the second result of TwoInt() (T, int) must be an error or a bool, got int"},
		{fn: "None", want: ResultShapeInvalid, wantErr: "method None() has no results; it should have one or two"},
		{fn: "Three", want: ResultShapeInvalid, wantErr: "method Three() (T, T, error) has 3 results; it should have one or two"},
	} {
		t.Run(tt.fn, func(t *testing.T) {
			got, err := classifyNestedCallResultShape(tt.fn, funcSignature(t, pkg, tt.fn), typeQualifier(pkg))
			assert.Equal(t, tt.want, got, "classifyNestedCallResultShape(%s)", tt.fn)
			assertErrorMessage(t, tt.wantErr, err, "classifyNestedCallResultShape(%s)", tt.fn)
		})
	}
}
