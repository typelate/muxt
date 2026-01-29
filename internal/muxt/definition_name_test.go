package muxt

import (
	"html/template"
	"testing"

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
			Out: "PeachPearPlumIndexByPeachIDAndPearID",
			In:  "/peach/{peachID}/pear/{pearID}/plum/{$}",
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
			pat, err, match := newDefinition(template.Must(template.New(tt.In).Parse(``)))
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
