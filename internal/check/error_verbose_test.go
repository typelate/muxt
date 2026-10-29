package check_test

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/check"
)

func TestCallError_VerboseError(t *testing.T) {
	// build sig: func(int, string) string
	intType := types.Universe.Lookup("int").Type()
	stringType := types.Universe.Lookup("string").Type()
	params := types.NewTuple(
		types.NewVar(token.NoPos, nil, "n", intType),
		types.NewVar(token.NoPos, nil, "s", stringType),
	)
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", stringType))
	sig := types.NewSignatureType(nil, nil, nil, params, results, false)

	e := &check.CallError{
		Name:      "Greet",
		Signature: sig,
		ArgTypes:  []types.Type{stringType, intType},
		Cause:     fmt.Errorf("argument 0 has type string expected int"),
	}

	require.Equal(t, "argument 0 has type string expected int", e.Error())

	verbose := e.VerboseError()
	require.Contains(t, verbose, "argument 0 has type string expected int")
	require.Contains(t, verbose, "signature: Greet(n int, s string) string")
	require.Contains(t, verbose, "[0] string")
	require.Contains(t, verbose, "[1] int")
	require.True(t, strings.Count(verbose, "\n") > 0, "verbose error should have multiple lines")
}

func TestCallError_VerboseError_AnonymousName(t *testing.T) {
	intType := types.Universe.Lookup("int").Type()
	params := types.NewTuple(types.NewVar(token.NoPos, nil, "", intType))
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", intType))
	sig := types.NewSignatureType(nil, nil, nil, params, results, false)

	e := &check.CallError{
		Signature: sig,
		ArgTypes:  []types.Type{intType},
		Cause:     fmt.Errorf("wrong number of args expected 1 but got 1"),
	}
	verbose := e.VerboseError()
	require.Contains(t, verbose, "signature: func(int) int")
}

func TestCallError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("boom")
	e := &check.CallError{Cause: cause}
	require.True(t, errors.Is(e, cause))
}

func TestIdentifierError_VerboseError_NamedType(t *testing.T) {
	// build a Named type wrapping struct{ Field string }
	stringType := types.Universe.Lookup("string").Type()
	field := types.NewField(token.NoPos, nil, "Field", stringType, false)
	st := types.NewStruct([]*types.Var{field}, []string{""})
	tn := types.NewTypeName(token.NoPos, nil, "Bar", nil)
	named := types.NewNamed(tn, st, nil)

	e := &check.IdentifierError{
		Identifier: "Missing",
		Type:       named,
		Cause:      fmt.Errorf("Missing not found on Bar"),
	}

	require.Equal(t, "Missing not found on Bar", e.Error())

	verbose := e.VerboseError()
	require.Contains(t, verbose, "Missing not found on Bar")
	require.Contains(t, verbose, "type: Bar")
	require.Contains(t, verbose, "underlying: struct{Field string}")
}

func TestIdentifierError_VerboseError_BasicType(t *testing.T) {
	// for a basic type (int), underlying == itself, so no underlying line.
	e := &check.IdentifierError{
		Identifier: "Foo",
		Type:       types.Universe.Lookup("int").Type(),
		Cause:      fmt.Errorf("Foo not found on int"),
	}
	verbose := e.VerboseError()
	require.Contains(t, verbose, "type: int")
	require.NotContains(t, verbose, "underlying:")
}

func TestIdentifierError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("boom")
	e := &check.IdentifierError{Cause: cause}
	require.True(t, errors.Is(e, cause))
}

func TestFormatVerbose_NoVerboseFallsBackToError(t *testing.T) {
	err := fmt.Errorf("plain")
	require.Equal(t, "plain", check.FormatVerbose(err))
}

func TestFormatVerbose_NilReturnsEmpty(t *testing.T) {
	require.Equal(t, "", check.FormatVerbose(nil))
}

func TestFormatVerbose_PrefersVerboseLeaf(t *testing.T) {
	stringType := types.Universe.Lookup("string").Type()
	intType := types.Universe.Lookup("int").Type()
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "x", intType)),
		types.NewTuple(types.NewVar(token.NoPos, nil, "", stringType)),
		false)

	e := &check.CallError{
		Name:      "F",
		Signature: sig,
		ArgTypes:  []types.Type{stringType},
		Cause:     fmt.Errorf("argument 0 has type string expected int"),
	}
	out := check.FormatVerbose(e)
	require.Contains(t, out, "signature: F(x int) string")
}

func TestFormatVerbose_JoinedErrors(t *testing.T) {
	stringType := types.Universe.Lookup("string").Type()
	intType := types.Universe.Lookup("int").Type()
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "x", intType)),
		types.NewTuple(types.NewVar(token.NoPos, nil, "", stringType)),
		false)

	a := &check.CallError{
		Name:      "A",
		Signature: sig,
		ArgTypes:  []types.Type{stringType},
		Cause:     fmt.Errorf("first failure"),
	}
	b := &check.IdentifierError{
		Identifier: "Missing",
		Type:       intType,
		Cause:      fmt.Errorf("second failure"),
	}
	joined := errors.Join(a, b)
	out := check.FormatVerbose(joined)

	require.Contains(t, out, "first failure")
	require.Contains(t, out, "signature: A(x int) string")
	require.Contains(t, out, "second failure")
	require.Contains(t, out, "type: int")
	// Two verbose blocks separated by a blank line.
	require.Contains(t, out, "\n\n")
}

// Confirm CallError and IdentifierError satisfy the VerboseErrorer interface
// at compile time. (Tests fail to build if they don't.)
var (
	_ check.VerboseErrorer = (*check.CallError)(nil)
	_ check.VerboseErrorer = (*check.IdentifierError)(nil)
)
