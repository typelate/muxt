package muxt

import (
	"testing"

	"github.com/typelate/muxt/internal/template/html"

	"github.com/stretchr/testify/require"
)

func TestTemplate_generateEndpointPatternIdentifier(t *testing.T) {
	for _, tt := range []struct {
		In  string
		Out string
	}{
		{
			Out: "ReadIndex",
			In:  "GET /",
		},
		{
			Out: "ReadArticle",
			In:  "GET /article",
		},
		{
			Out: "CreateArticle",
			In:  "POST /article",
		},
		{
			Out: "UpdateArticle",
			In:  "PATCH /article",
		},
		{
			Out: "ReplaceArticle",
			In:  "PUT /article",
		},
		{
			Out: "DeleteArticle",
			In:  "DELETE /article",
		},
		{
			Out: "Article",
			In:  "/article",
		},
		{
			Out: "PeachPear",
			In:  "/peach/pear",
		},
		{
			Out: "PeachPearByPeachID",
			In:  "/peach/{peachID}/pear",
		},
		{
			Out: "PeachPearByPeachIDAndPearID",
			In:  "/peach/{peachID}/pear/{pearID}",
		},
		{
			Out: "PeachPearPlumByPeachIDAndPearID",
			In:  "/peach/{peachID}/pear/{pearID}/plum",
		},
		{
			Out: "PeachPearPlumExactByPeachIDAndPearID",
			In:  "/peach/{peachID}/pear/{pearID}/plum/{$}",
		},
		{
			// Regression: GET / and GET /{$} are distinct routes in
			// http.ServeMux and must produce distinct generated identifiers.
			Out: "ReadExact",
			In:  "GET /{$}",
		},
		{
			Out: "PlumPrune",
			In:  "/plum-prune",
		},
		{
			Out: "ReadX",
			In:  "GET /x Handler()",
		},
		{
			Out: "ReadExampleComIndex",
			In:  "GET example.com/ X()",
		},
		{
			Out: "CreateABCDExampleComIndex",
			In:  "POST a.b.c.d.example.com/ X()",
		},
		{
			Out: "CreateExampleComPeach",
			In:  "POST example.com/peach X()",
		},
	} {
		t.Run(tt.Out, func(t *testing.T) {
			pat, err, match := newDefinition(html.Must(html.New(tt.In).Parse(``)))
			require.True(t, match)
			require.NoError(t, err)
			require.Equal(t, tt.Out, pat.generateEndpointPatternIdentifier(nil))
		})
	}

	t.Run("non standard http method", func(t *testing.T) {
		def := Definition{
			method: "CONNECT",
			path:   "/",
		}
		require.Equal(t, "ConnectIndex", def.generateEndpointPatternIdentifier(nil))
	})
}

// TestStaticAssetsPathIdentifier states the TemplateRoutePaths method name of
// the static assets route: the name a call-less GET route with that path
// gets.
func TestStaticAssetsPathIdentifier(t *testing.T) {
	callLess := func(prefix string) string {
		def, err, match := newDefinition(html.Must(html.New("GET " + prefix).Parse(``)))
		require.True(t, match)
		require.NoError(t, err)
		defs := []Definition{def}
		calculateIdentifiers(defs)
		ident, err := defs[0].ExportedPathIdentifier()
		require.NoError(t, err)
		return ident
	}
	for _, tt := range []struct {
		prefix string
		want   string
	}{
		{prefix: "/static/", want: "ReadStatic"},
		{prefix: "/public/files/", want: "ReadPublicFiles"},
		{prefix: "/static/v1/", want: "ReadStaticV1"},
		{prefix: "/assets/img/", want: "ReadAssetsImg"},
		{prefix: "/my-assets/", want: callLess("/my-assets/")},
	} {
		if got := StaticAssetsPathIdentifier(tt.prefix); got != tt.want {
			t.Errorf("StaticAssetsPathIdentifier(%q) = %q, want %q", tt.prefix, got, tt.want)
		}
	}
}
