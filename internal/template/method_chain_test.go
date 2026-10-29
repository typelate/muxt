package template_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// methodChainOuter exposes a zero-argument method returning an interface
// value, the shape muxt's generated TemplateData.Result() takes when a
// handler method has an inferred (any) result.
type methodChainOuter struct{}

func (methodChainOuter) Result() any { return methodChainInner{Name: "PEACH"} }

type methodChainInner struct {
	Name string
}

// TestMethodInNonFinalChainPosition is a regression test: a zero-argument
// method in a non-final field-chain position ({{.Result.Name}}) must be
// callable. Previously evalFieldChain passed nil args for non-final fields,
// so evalCall computed len(args)-1 == -1 fixed arguments and rejected the
// zero-parameter method with "wrong number of args for Result: want 0 got -1".
func TestMethodInNonFinalChainPosition(t *testing.T) {
	got := execText(t, "chain", `{{.Result.Name}}`, methodChainOuter{})
	assert.Equal(t, "PEACH", got)
}

// pointerFieldOuter exposes a method returning a pointer to a struct, the
// shape muxt's generated TemplateData.Request() takes (*http.Request).
type pointerFieldOuter struct{}

func (pointerFieldOuter) Request() *pointerFieldInner {
	return &pointerFieldInner{Pattern: "GET /"}
}

type pointerFieldInner struct {
	Pattern string
}

// TestFieldOnPointerReceiver is a regression test: a struct field accessed
// through a pointer value ({{.Request.Pattern}}, where Request returns
// *pointerFieldInner) must be reachable. evalField's struct-field branch
// switched on receiver.Kind() without dereferencing the pointer first, so
// it always fell through to "can't evaluate field Pattern in type
// *htt_test.pointerFieldInner" for any pointer-typed field host — notably
// muxt-generated TemplateData.Request(), which returns *http.Request.
func TestFieldOnPointerReceiver(t *testing.T) {
	got := execText(t, "chain", `{{.Request.Pattern}}`, pointerFieldOuter{})
	assert.Equal(t, "GET /", got)
}

// TestZeroArgMethodAsBareArgument is a regression test: a zero-argument
// method used as one argument among several ({{printf "%v" .Result}}, the
// shape muxt-generated code takes for an untyped Result) must be callable.
// evalArg's *FieldNode/*VariableNode/*ChainNode cases passed nil as the
// args slice for the field/method chain they evaluate; evalCall computes
// fixed-arg count as len(args)-1, so nil (len 0) computed -1 and rejected
// the zero-parameter method with "wrong number of args for Result: want 0
// got -1" - the same failure TestMethodInNonFinalChainPosition covers for
// a non-final chain position, but here the field is the chain's only (and
// so also final) segment, reached through evalArg rather than through the
// non-final loop in evalFieldChain.
func TestZeroArgMethodAsBareArgument(t *testing.T) {
	got := execText(t, "chain", `{{printf "%v" .Result}}`, methodChainOuter{})
	assert.Equal(t, "{PEACH}", got)
}

// pointerStringer implements fmt.Stringer on a pointer receiver only, the
// shape muxt-generated TemplateData takes (its String method returns "").
type pointerStringer struct{ Field string }

func (p *pointerStringer) String() string { return "" }

// TestPointerReceiverStringer is a regression test: printing a value whose
// Stringer is implemented on a pointer receiver must call that String
// method, not fall back to a raw %v of the dereferenced struct.
// printableValue dereferenced any pointer before checking whether the
// value implements error/fmt.Stringer; the dereferenced (non-pointer)
// value never implements a pointer-receiver method, so the check always
// failed and every pointer-to-Stringer value printed as its raw struct
// literal instead of via String().
func TestPointerReceiverStringer(t *testing.T) {
	got := execText(t, "stringer", `{{.}}`, &pointerStringer{Field: "should not appear"})
	assert.Equal(t, "", got)
}
