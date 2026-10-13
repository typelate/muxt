package load_test

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

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

	t.Run("a pointer type parses from text only if a pointer to it does", func(t *testing.T) {
		var timeType types.Type
		for _, imported := range pl[0].Types.Imports() {
			if imported.Path() == "time" {
				timeType = imported.Scope().Lookup("Time").Type()
			}
		}
		require.NotNil(t, timeType)
		assert.False(t, std.TextUnmarshaler(types.NewPointer(timeType)), "TextUnmarshaler(*time.Time) asks about **time.Time")
		assert.True(t, std.TextMarshaler(types.NewPointer(timeType)), "TextMarshaler(*time.Time)")
	})

	t.Run("a package the load did not reach", func(t *testing.T) {
		// Only the package itself, which imports nothing: encoding/json is
		// reached through html/template's imports in a real package.
		bare := t.TempDir()
		_, err := load.StandardLibrary(loadtest.Package(t, bare, "example.com/bare", map[string]string{"bare.go": "package bare\n"})[:1]).RawJSON()
		assert.EqualError(t, err, `could not find package "encoding/json" for RawMessage`)
	})
}

// TestStandardLibraryFromTestVariants states that a load with tests, which
// reports a package both as written and compiled with its test files,
// answers with the package as written: the one the package under test
// imports.
func TestStandardLibraryFromTestVariants(t *testing.T) {
	pl := loadtest.Package(t, t.TempDir(), "example.com/server", map[string]string{"server.go": "package server\n"})
	var written *types.Package
	for _, pkg := range pl {
		if pkg.PkgPath == "net/http" {
			written = pkg.Types
		}
	}
	require.NotNil(t, written, "loadtest.Package() loads net/http")

	// A stand-in for "net/http [net/http.test]": the same path, another
	// *types.Package, listed first as testVariantsFirst lists it.
	variant := types.NewPackage("net/http", "http")
	variant.Scope().Insert(types.NewTypeName(token.NoPos, variant, "Request", types.NewStruct(nil, nil)))
	testMain := types.NewPackage("net/http.test", "main")
	withTests := append([]*packages.Package{
		{ID: "net/http [net/http.test]", PkgPath: "net/http", Types: variant},
		{ID: "net/http.test", PkgPath: "net/http.test", Types: testMain},
	}, pl...)

	request, err := load.StandardLibrary(withTests).ScopeType(muxt.TemplateNameScopeIdentifierHTTPRequest)
	require.NoError(t, err, "ScopeType(request)")
	want := types.NewPointer(written.Scope().Lookup("Request").Type())
	assert.True(t, types.Identical(want, request), "ScopeType(request) = %s from the test variant, want the package as written", request)
}

// TestStandardLibraryDeclarationsThatAreNotTypes states that a name the
// standard library declares as something other than a type is not
// answered as one.
func TestStandardLibraryDeclarationsThatAreNotTypes(t *testing.T) {
	io := types.NewPackage("io", "io")
	io.Scope().Insert(types.NewVar(token.NoPos, io, "Reader", types.Typ[types.Int]))
	encoding := types.NewPackage("encoding", "encoding")
	encoding.Scope().Insert(types.NewFunc(token.NoPos, encoding, "TextMarshaler", types.NewSignatureType(nil, nil, nil, nil, nil, false)))
	encoding.Scope().Insert(types.NewTypeName(token.NoPos, encoding, "TextUnmarshaler", types.Typ[types.Int]))
	std := load.StandardLibrary([]*packages.Package{
		{ID: "io", PkgPath: "io", Types: io},
		{ID: "encoding", PkgPath: "encoding", Types: encoding},
	})

	_, err := std.ScopeType(muxt.TemplateNameScopeIdentifierRequestBody)
	assert.EqualError(t, err, `package "io" declares no type Reader`)
	assert.False(t, std.TextMarshaler(types.Typ[types.String]), "TextMarshaler when encoding.TextMarshaler is a func")
	assert.False(t, std.TextUnmarshaler(types.Typ[types.String]), "TextUnmarshaler when encoding.TextUnmarshaler is not an interface")
}
