package muxt

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRewrittenArgumentsKeepTheirPositions states that an argument rewritten
// to its explicit spelling still spans what was written, so an error about
// it marks the right part of the template name.
func TestRewrittenArgumentsKeepTheirPositions(t *testing.T) {
	t.Run("signals", func(t *testing.T) {
		call := mustParseCall(t, `F(ctx, signals)`)
		written := call.Args[1]
		require.True(t, rewriteSignalsArguments(call, nil), "rewriteSignalsArguments()")
		rewritten, ok := call.Args[1].(*ast.CallExpr)
		require.True(t, ok, "signals is rewritten to a call, got %T", call.Args[1])
		assert.Equal(t, written.Pos(), rewritten.Pos(), "Pos() of the rewritten signals")
		assert.Equal(t, written.End(), rewritten.End(), "End() of the rewritten signals")
		assert.Equal(t, written.Pos(), rewritten.Args[0].Pos(), "Pos() of the rewritten signals body")
	})
	t.Run("unmarshalForm", func(t *testing.T) {
		call := mustParseCall(t, `F(ctx, unmarshalForm(body))`)
		written := call.Args[1]
		rewriteBodyFormWrappers(call)
		rewritten, ok := call.Args[1].(*ast.Ident)
		require.True(t, ok, "unmarshalForm(body) is rewritten to an identifier, got %T", call.Args[1])
		assert.Equal(t, written.Pos(), rewritten.Pos(), "Pos() of the rewritten unmarshalForm(body)")
	})
}

func TestScanBodyBindings(t *testing.T) {
	for _, tt := range []struct {
		name string
		call string
		want bodyBindings
	}{
		{name: "no arguments", call: `F()`},
		{name: "body identifier", call: `F(body)`, want: bodyBindings{reads: 1}},
		{name: "form identifier", call: `F(form)`, want: bodyBindings{hasForm: true}},
		{name: "multipart identifier", call: `F(multipart)`, want: bodyBindings{hasMultipart: true}},
		{name: "unrelated identifiers", call: `F(ctx, request, 5)`},
		{name: "unmarshalJSON wrapper", call: `F(unmarshalJSON(body))`, want: bodyBindings{reads: 1}},
		{name: "unmarshalForm wrapper is the form binding", call: `F(unmarshalForm(body))`, want: bodyBindings{hasForm: true}},
		{name: "two reads", call: `F(body, unmarshalJSON(body))`, want: bodyBindings{reads: 2}},
		{name: "nested call", call: `F(G(body, form))`, want: bodyBindings{reads: 1, hasForm: true}},
		{name: "nested multipart", call: `F(G(H(multipart)))`, want: bodyBindings{hasMultipart: true}},
		{name: "nested and direct", call: `F(form, G(multipart, body))`, want: bodyBindings{reads: 1, hasForm: true, hasMultipart: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, scanBodyBindings(mustParseCall(t, tt.call)), "scanBodyBindings(%s)", tt.call)
		})
	}
}
