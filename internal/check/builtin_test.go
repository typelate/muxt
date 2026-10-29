package check_test

import (
	"fmt"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

var _ check.VerboseErrorer = (*check.BuiltinError)(nil)

func TestBuiltinDoc(t *testing.T) {
	and, ok := check.BuiltinDoc("and")
	require.True(t, ok)
	assert.Contains(t, and.Usage, "and")
	assert.Contains(t, and.Note, "Short-circuit", "and documents short-circuit evaluation")

	or, ok := check.BuiltinDoc("or")
	require.True(t, ok)
	assert.Contains(t, or.Note, "Short-circuit")

	// A built-in with no special semantics has usage but no note.
	length, ok := check.BuiltinDoc("len")
	require.True(t, ok)
	assert.NotEmpty(t, length.Usage)
	assert.Empty(t, length.Note)

	_, ok = check.BuiltinDoc("definitely-not-a-builtin")
	assert.False(t, ok)
}

func TestBuiltinError_VerboseError(t *testing.T) {
	doc, _ := check.BuiltinDoc("and")
	e := &check.BuiltinError{
		Name:     "and",
		Usage:    doc.Usage,
		Note:     doc.Note,
		ArgTypes: []types.Type{types.Typ[types.String], types.Typ[types.Int]},
		Cause:    fmt.Errorf("built-in and expects at least one argument got 0"),
	}

	// The short form is just the cause.
	assert.Equal(t, "built-in and expects at least one argument got 0", e.Error())

	verbose := e.VerboseError()
	assert.Contains(t, verbose, "built-in and expects at least one argument got 0")
	assert.Contains(t, verbose, "usage: and")
	assert.Contains(t, verbose, "arguments:")
	assert.Contains(t, verbose, "[0] string")
	assert.Contains(t, verbose, "[1] int")
	assert.Contains(t, verbose, "note: ")
	assert.Contains(t, verbose, "Short-circuit")
}

// The ordered comparisons take exactly two arguments; only eq is variadic.
func TestComparisonBuiltinArgCounts(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	// data has an int field N so piped forms like {{.N | lt 5}} type-check.
	data := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "N", types.Typ[types.Int], false),
	}, nil)
	checkTemplate := func(src string) error {
		tp, err := template.New[*types.Signature]("main").Parse(src)
		require.NoError(t, err)
		global := check.NewGlobal(testPkg.Types, testPkg.Fset, tp, check.DefaultFunctions(testPkg.Types))
		tree, ok := tp.FindTree("main")
		require.True(t, ok)
		return check.Execute(global, tree, data)
	}

	// lt (and its siblings) with three arguments is rejected — before, this
	// slipped past the checker and only failed at runtime.
	err := checkTemplate(`{{lt 1 2 3}}`)
	var be *check.BuiltinError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "lt", be.Name)
	assert.Contains(t, be.Error(), "expects two arguments got 3")

	// eq stays variadic: arg1 == arg2 || arg1 == arg3.
	require.NoError(t, checkTemplate(`{{eq 1 2 3}}`))

	// A piped value is the comparison's final argument: {{.N | lt 5}} is
	// lt(5, .N) — two arguments, so it must pass the exactly-two check.
	require.NoError(t, checkTemplate(`{{.N | lt 5}}`), "piped value counts as the second argument")
	require.NoError(t, checkTemplate(`{{lt 5 .N}}`), "the un-piped equivalent")

	// The pipe makes the count explicit both ways: {{.N | lt}} is lt(.N) —
	// one argument — and {{.N | lt 5 6}} is lt(5, 6, .N) — three.
	require.Error(t, checkTemplate(`{{.N | lt}}`), "one argument is rejected")
	require.Error(t, checkTemplate(`{{.N | lt 5 6}}`), "three arguments (two explicit + pipe) is rejected")
}

// builtInCheck wires real built-in failures to *BuiltinError so they render
// verbosely like function-call errors.
func TestBuiltInCheckReturnsBuiltinError(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	tmpl, err := template.New[*types.Signature]("main").Parse("{{len .N}}")
	require.NoError(t, err)
	dt := types.NewStruct([]*types.Var{
		types.NewField(0, testPkg.Types, "N", types.Typ[types.Int], false),
	}, nil)

	global := check.NewGlobal(testPkg.Types, testPkg.Fset, tmpl, check.DefaultFunctions(testPkg.Types))
	tree, ok := tmpl.FindTree("main")
	require.True(t, ok)
	execErr := check.Execute(global, tree, dt)
	require.Error(t, execErr)

	var be *check.BuiltinError
	require.ErrorAs(t, execErr, &be, "a bad built-in call is a *BuiltinError")
	assert.Equal(t, "len", be.Name)

	verbose := be.VerboseError()
	assert.Contains(t, verbose, "usage: len")
	assert.Contains(t, verbose, "[0] int", "the offending argument type is shown")

	// FormatVerbose (the CLI/LSP entry point) renders it verbosely.
	assert.Contains(t, check.FormatVerbose(execErr), "usage: len")
}
