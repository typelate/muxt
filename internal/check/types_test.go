package check_test

import "math"

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
		D T
	}
)
