package check

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template"
)

func TestErrorDetailedError(t *testing.T) {
	intType := types.Universe.Lookup("int").Type()
	stringType := types.Universe.Lookup("string").Type()
	params := types.NewTuple(
		types.NewVar(token.NoPos, nil, "n", intType),
		types.NewVar(token.NoPos, nil, "s", stringType),
	)
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", stringType))
	sig := types.NewSignatureType(nil, nil, nil, params, results, false)

	callErr := &CallError{
		Name:      "Test",
		Signature: sig,
		ArgTypes:  []types.Type{stringType, intType},
		Cause:     fmt.Errorf("argument 0 has type string expected int"),
	}

	tree := &template.Tree{
		Name:      "test",
		ParseName: "test.gohtml",
	}
	node := &template.TextNode{Text: []byte("test")}

	e := wrapError(ErrorTypeCallArguments, tree, node, callErr)

	var buf strings.Builder
	require.NoError(t, e.DetailedError(&buf, nil))
	assert.Equal(t, strings.Join([]string{
		e.Error(),
		"",
		"  signature: Test(n int, s string) string",
		"  arguments:",
		"    [0] string",
		"    [1] int",
	}, "\n"), buf.String(), "DetailedError renders the compact line, then the callee signature and the argument types")
}

// TestErrorDetailedError_interface checks that an interface receiver renders
// as an interface declaration. Upstream reaches this through {{.Missing}}
// on an interface; here a field access on an interface value is a warning
// (WarnInterfaceFieldAccess), not an error, so the error is built directly.
func TestErrorDetailedError_interface(t *testing.T) {
	pkg := types.NewPackage("example.com/web", "web")
	greetSig := types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.String])), false)
	iface := types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, pkg, "Greet", greetSig),
	}, nil)
	iface.Complete()
	greeter := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Greeter", nil), iface, nil)

	render := notFoundMessage("Missing", greeter)
	e := wrapError(ErrorTypeFieldOrMethodNotFound, nil, nil, &IdentifierError{
		Identifier: "Missing",
		Type:       greeter,
		Cause:      fmt.Errorf("%s", render(fullTypeFormat(nil))),
		render:     render,
	}).withX(greeter)

	var sb strings.Builder
	require.NoError(t, e.DetailedError(&sb, func(p *types.Package) string { return p.Name() }))
	assert.Equal(t, strings.Join([]string{
		"field or method Missing not found on web.Greeter",
		"",
		"  type Greeter interface {",
		"    Greet() string",
		"  }",
	}, "\n"), sb.String())
}
