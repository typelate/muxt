package check

import (
	"bytes"
	"fmt"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template"
)

func TestErrorDetailedError(t *testing.T) {
	// Construct a CallError (following the existing test pattern)
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

	// Construct a *Error with a minimal Tree and TextNode
	tree := &template.Tree{
		Name:      "test",
		ParseName: "test.gohtml",
	}
	node := &template.TextNode{Text: []byte("test")}

	e := wrapError(tree, node, callErr)

	var buf bytes.Buffer
	require.NoError(t, e.DetailedError(&buf, nil))
	assert.Equal(t, e.VerboseError(), buf.String())
}
