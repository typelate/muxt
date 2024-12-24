package check_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"text/template"
)

type Void struct{}

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

func (MethodWithIntParam) F(int) (_ Void) { return }

type MethodWithInt8Param struct{}

func (MethodWithInt8Param) F(int8) (_ Void) { return }

type MethodWithInt16Param struct{}

func (MethodWithInt16Param) F(int16) (_ Void) { return }

type MethodWithInt32Param struct{}

func (MethodWithInt32Param) F(int32) (_ Void) { return }

type MethodWithInt64Param struct{}

func (MethodWithInt64Param) F(int64) (_ Void) { return }

type MethodWithUintParam struct{}

func (MethodWithUintParam) F(uint) (_ Void) { return }

type MethodWithUint8Param struct{}

func (MethodWithUint8Param) F(uint8) (_ Void) { return }

type MethodWithUint16Param struct{}

func (MethodWithUint16Param) F(uint16) (_ Void) { return }

type MethodWithUint32Param struct{}

func (MethodWithUint32Param) F(uint32) (_ Void) { return }

type MethodWithUint64Param struct{}

func (MethodWithUint64Param) F(uint64) (_ Void) { return }

type MethodWithBoolParam struct{}

func (MethodWithBoolParam) F(bool) (_ Void) { return }

type MethodWithFloat64Param struct{}

func (MethodWithFloat64Param) F(float64) (_ Void) { return }

type MethodWithFloat32Param struct{}

func (MethodWithFloat32Param) F(float32) (_ Void) { return }

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

func (MethodWithKeyValForSlices) F(int, float64) (_ Void) { return }

type MethodWithKeyValForArray struct {
	Numbers [2]float64
}

func (MethodWithKeyValForArray) F(int, float64) (_ Void) { return }

type MethodWithKeyValForMap struct {
	Numbers map[int16]float32
}

func (MethodWithKeyValForMap) F(int16, float32) (_ Void) { return }

func square(n int) int {
	return n * n
}

func ceil(n float64) int {
	return int(math.Ceil(n))
}

func expectInt(n int) int { return n }

func expectFloat64(n float64) float64 { return n }

func expectString(s string) string { return s }

func expectInt8(n int8) int8 { return n }

func expectFloat32(n float32) float32 { return n }

func expectComplex64(n complex64) complex64 { return n }

func expectComplex128(n complex128) complex128 { return n }

type (
	LetterChainA struct {
		A LetterChainB
	}
	LetterChainB struct {
		B LetterChainC
	}
	LetterChainC struct {
		C LetterChainD
	}
	LetterChainD struct {
		D Void
	}
)

// T has lots of interesting pieces to use to test execution.
type T struct {
	// Basics
	True        bool
	I           int
	U16         uint16
	X, S        string
	FloatZero   float64
	ComplexZero complex128
	// Nested structs.
	U *U
	// Struct with String method.
	V0     V
	V1, V2 *V
	// Struct with Error method.
	W0     W
	W1, W2 *W
	// Slices
	SI      []int
	SICap   []int
	SIEmpty []int
	SB      []bool
	// Arrays
	AI [3]int
	// Maps
	MSI      map[string]int
	MSIone   map[string]int // one element, for deterministic output
	MSIEmpty map[string]int
	MXI      map[any]int
	MII      map[int]int
	MI32S    map[int32]string
	MI64S    map[int64]string
	MUI32S   map[uint32]string
	MUI64S   map[uint64]string
	MI8S     map[int8]string
	MUI8S    map[uint8]string
	SMSI     []map[string]int
	// Empty interfaces; used to see if we can dig inside one.
	Empty0 any // nil
	Empty1 any
	Empty2 any
	Empty3 any
	Empty4 any
	// Non-empty interfaces.
	NonEmptyInterface         I
	NonEmptyInterfacePtS      *I
	NonEmptyInterfaceNil      I
	NonEmptyInterfaceTypedNil I
	// Stringer.
	Str fmt.Stringer
	Err error
	// Pointers
	PI  *int
	PS  *string
	PSI *[]int
	NIL *int
	// Function (not method)
	BinaryFunc             func(string, string) string
	VariadicFunc           func(...string) string
	VariadicFuncInt        func(int, ...string) string
	NilOKFunc              func(*int) bool
	ErrFunc                func() (string, error)
	PanicFunc              func() string
	TooFewReturnCountFunc  func()
	TooManyReturnCountFunc func() (string, error, int)
	InvalidReturnTypeFunc  func() (string, bool)
	// Template to test evaluation of templates.
	Tmpl *template.Template
	// Unexported field; cannot be accessed by template.
	unexported int
}

type S []string

func (S) Method0() string {
	return "M0"
}

type U struct {
	V string
}

type V struct {
	j int
}

func (v *V) String() string {
	if v == nil {
		return "nilV"
	}
	return fmt.Sprintf("<%d>", v.j)
}

type W struct {
	k int
}

func (w *W) Error() string {
	if w == nil {
		return "nilW"
	}
	return fmt.Sprintf("[%d]", w.k)
}

var siVal = I(S{"a", "b"})

var tVal = &T{
	True:   true,
	I:      17,
	U16:    16,
	X:      "x",
	S:      "xyz",
	U:      &U{"v"},
	V0:     V{6666},
	V1:     &V{7777}, // leave V2 as nil
	W0:     W{888},
	W1:     &W{999}, // leave W2 as nil
	SI:     []int{3, 4, 5},
	SICap:  make([]int, 5, 10),
	AI:     [3]int{3, 4, 5},
	SB:     []bool{true, false},
	MSI:    map[string]int{"one": 1, "two": 2, "three": 3},
	MSIone: map[string]int{"one": 1},
	MXI:    map[any]int{"one": 1},
	MII:    map[int]int{1: 1},
	MI32S:  map[int32]string{1: "one", 2: "two"},
	MI64S:  map[int64]string{2: "i642", 3: "i643"},
	MUI32S: map[uint32]string{2: "u322", 3: "u323"},
	MUI64S: map[uint64]string{2: "ui642", 3: "ui643"},
	MI8S:   map[int8]string{2: "i82", 3: "i83"},
	MUI8S:  map[uint8]string{2: "u82", 3: "u83"},
	SMSI: []map[string]int{
		{"one": 1, "two": 2},
		{"eleven": 11, "twelve": 12},
	},
	Empty1:                    3,
	Empty2:                    "empty2",
	Empty3:                    []int{7, 8},
	Empty4:                    &U{"UinEmpty"},
	NonEmptyInterface:         &T{X: "x"},
	NonEmptyInterfacePtS:      &siVal,
	NonEmptyInterfaceTypedNil: (*T)(nil),
	Str:                       bytes.NewBuffer([]byte("foozle")),
	Err:                       errors.New("erroozle"),
	PI:                        newInt(23),
	PS:                        newString("a string"),
	PSI:                       newIntSlice(21, 22, 23),
	BinaryFunc:                func(a, b string) string { return fmt.Sprintf("[%s=%s]", a, b) },
	VariadicFunc:              func(s ...string) string { return fmt.Sprint("<", strings.Join(s, "+"), ">") },
	VariadicFuncInt:           func(a int, s ...string) string { return fmt.Sprint(a, "=<", strings.Join(s, "+"), ">") },
	NilOKFunc:                 func(s *int) bool { return s == nil },
	ErrFunc:                   func() (string, error) { return "bla", nil },
	PanicFunc:                 func() string { panic("test panic") },
	TooFewReturnCountFunc:     func() {},
	TooManyReturnCountFunc:    func() (string, error, int) { return "", nil, 0 },
	InvalidReturnTypeFunc:     func() (string, bool) { return "", false },
	Tmpl:                      template.Must(template.New("x").Parse("test template")), // "x" is the value of .X
}

var tSliceOfNil = []*T{nil}

// A non-empty interface.
type I interface {
	Method0() string
}

var iVal I = tVal

// Helpers for creation.
func newInt(n int) *int {
	return &n
}

func newString(s string) *string {
	return &s
}

func newIntSlice(n ...int) *[]int {
	p := new([]int)
	*p = make([]int, len(n))
	copy(*p, n)
	return p
}

// Simple methods with and without arguments.
func (t *T) Method0() string {
	return "M0"
}

func (t *T) Method1(a int) int {
	return a
}

func (t *T) Method2(a uint16, b string) string {
	return fmt.Sprintf("Method2: %d %s", a, b)
}

func (t *T) Method3(v any) string {
	return fmt.Sprintf("Method3: %v", v)
}

func (t *T) Copy() *T {
	n := new(T)
	*n = *t
	return n
}

func (t *T) MAdd(a int, b []int) []int {
	v := make([]int, len(b))
	for i, x := range b {
		v[i] = x + a
	}
	return v
}

var myError = errors.New("my error")

// MyError returns a value and an error according to its argument.
func (t *T) MyError(error bool) (bool, error) {
	if error {
		return true, myError
	}
	return false, nil
}

// A few methods to test chaining.
func (t *T) GetU() *U {
	return t.U
}

func (u *U) TrueFalse(b bool) string {
	if b {
		return "true"
	}
	return ""
}

func typeOf(arg any) string {
	return fmt.Sprintf("%T", arg)
}

func zeroArgs() string {
	return "zeroArgs"
}

func oneArg(a string) string {
	return "oneArg=" + a
}

func twoArgs(a, b string) string {
	return "twoArgs=" + a + b
}

func dddArg(a int, b ...string) string {
	return fmt.Sprintln(a, b)
}

// count returns a channel that will deliver n sequential 1-letter strings starting at "a"
func count(n int) chan string {
	if n == 0 {
		return nil
	}
	c := make(chan string)
	go func() {
		for i := 0; i < n; i++ {
			c <- "abcdefghijklmnop"[i : i+1]
		}
		close(c)
	}()
	return c
}

// vfunc takes a *V and a V
func vfunc(V, *V) string {
	return "vfunc"
}

// valueString takes a string, not a pointer.
func valueString(v string) string {
	return "value is ignored"
}

// returnInt returns an int
func returnInt() int {
	return 7
}

func add(args ...int) int {
	sum := 0
	for _, x := range args {
		sum += x
	}
	return sum
}

func echo(arg any) any {
	return arg
}

func makemap(arg ...string) map[string]string {
	if len(arg)%2 != 0 {
		panic("bad makemap")
	}
	m := make(map[string]string)
	for i := 0; i < len(arg); i += 2 {
		m[arg[i]] = arg[i+1]
	}
	return m
}

func stringer(s fmt.Stringer) string {
	return s.String()
}

func mapOfThree() any {
	return map[string]int{"three": 3}
}

func die() bool { panic("die") }

func print(in ...any) string {
	return fmt.Sprint(in...)
}

func println(in ...any) string {
	return fmt.Sprintln(in...)
}

func printf(f string, in ...any) string {
	return fmt.Sprintf(f, in...)
}

func not(in bool) bool { return !in }

func and(...any) bool { return false }

func or(...any) bool { return false }
