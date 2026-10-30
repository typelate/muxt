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
