package generate

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNamedTypeFrom(t *testing.T) {
	ssePkg := types.NewPackage("github.com/typelate/sse", "sse")
	sig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	messageOption := types.NewNamed(types.NewTypeName(token.NoPos, ssePkg, "MessageOption", nil), sig, nil)
	otherPkg := types.NewPackage("example.com/x", "x")
	aliased := types.NewAlias(types.NewTypeName(token.NoPos, otherPkg, "MyOption", nil), messageOption)

	assert.True(t, isNamedTypeFrom(messageOption, "github.com/typelate/sse", "MessageOption"))
	assert.True(t, isNamedTypeFrom(aliased, "github.com/typelate/sse", "MessageOption"),
		"an alias is identical to its target type and must be accepted")
	assert.False(t, isNamedTypeFrom(messageOption, "github.com/typelate/sse", "SourceOption"))
	assert.False(t, isNamedTypeFrom(types.Typ[types.String], "github.com/typelate/sse", "MessageOption"))
}
