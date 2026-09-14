package load_test

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/load/loadtest"
	"github.com/typelate/muxt/internal/muxt"
)

// TestStandardLibrary states what the one checker backed by the official
// standard library answers, which the mock checkers in other packages' tests
// stand in for.
func TestStandardLibrary(t *testing.T) {
	dir := t.TempDir()
	pl := loadtest.Package(t, dir, "example.com/server", map[string]string{
		"server.go": `package server

import (
	"html/template"
	"time"
)

type Plain struct{}

var (
	_ time.Time
	_ template.HTML
)
`,
	})
	std := load.StandardLibrary(pl)

	t.Run("the reserved identifiers", func(t *testing.T) {
		for identifier, want := range map[string]string{
			muxt.TemplateNameScopeIdentifierHTTPRequest:  "*net/http.Request",
			muxt.TemplateNameScopeIdentifierHTTPResponse: "net/http.ResponseWriter",
			muxt.TemplateNameScopeIdentifierContext:      "context.Context",
			muxt.TemplateNameScopeIdentifierForm:         "net/url.Values",
			muxt.TemplateNameScopeIdentifierMultipart:    "*mime/multipart.Form",
			muxt.TemplateNameScopeIdentifierRequestBody:  "io.Reader",
		} {
			tp, err := std.ScopeType(identifier)
			require.NoError(t, err, identifier)
			assert.Equal(t, want, types.TypeString(tp, nil), identifier)
		}
		_, err := std.ScopeType("lastEventID")
		assert.EqualError(t, err, "lastEventID is not a reserved argument identifier")
	})

	t.Run("the types a binding needs", func(t *testing.T) {
		fileHeader, err := std.FileHeader()
		require.NoError(t, err)
		assert.Equal(t, "*mime/multipart.FileHeader", types.TypeString(fileHeader, nil))
		rawJSON, err := std.RawJSON()
		require.NoError(t, err)
		assert.Equal(t, "encoding/json.RawMessage", types.TypeString(rawJSON, nil))
	})

	t.Run("text marshaling", func(t *testing.T) {
		var timeType types.Type
		for _, imported := range pl[0].Types.Imports() {
			if imported.Path() == "time" {
				timeType = imported.Scope().Lookup("Time").Type()
			}
		}
		require.NotNil(t, timeType)
		plain := pl[0].Types.Scope().Lookup("Plain").Type()
		assert.True(t, std.TextUnmarshaler(timeType), "a *time.Time parses from text")
		assert.True(t, std.TextMarshaler(timeType), "a time.Time formats as text")
		assert.False(t, std.TextUnmarshaler(plain))
		assert.False(t, std.TextMarshaler(plain))
	})

	t.Run("a package the load did not reach", func(t *testing.T) {
		// Only the package itself, which imports nothing: encoding/json is
		// reached through html/template's imports in a real package.
		bare := t.TempDir()
		_, err := load.StandardLibrary(loadtest.Package(t, bare, "example.com/bare", map[string]string{"bare.go": "package bare\n"})[:1]).RawJSON()
		assert.EqualError(t, err, `could not find package "encoding/json" for RawMessage`)
	})
}
