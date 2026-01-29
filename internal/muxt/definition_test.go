package muxt_test

import (
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/muxt"
)

func TestDefinitions(t *testing.T) {
	t.Run("when one of the template names is a malformed pattern", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "HEAD /"}}{{end}}`))
		_, err := muxt.Definitions(ts, "ts")
		require.Error(t, err)
	})
}

func TestCheckForDuplicatePatterns(t *testing.T) {
	t.Run("when the pattern is not unique", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET  / F1()"}}a{{end}} {{define "GET /  F2()"}}b{{end}}`))
		definitions, err := muxt.Definitions(ts, "ts")
		require.NoError(t, err)
		require.Len(t, definitions, 2)
		for _, def := range definitions {
			assert.Equal(t, "GET /", def.Pattern())
		}
		require.ErrorContains(t, muxt.CheckForDuplicatePatterns(definitions), `duplicate route pattern "GET /"`, "it should find the duplicate")
	})

	t.Run("ensure hosts are normalized", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET  example.com/ F1()"}}a{{end}} {{define "GET Example.COM/  F2()"}}b{{end}}`))
		definitions, err := muxt.Definitions(ts, "ts")
		require.NoError(t, err)
		require.Len(t, definitions, 2)
		for _, def := range definitions {
			assert.Equal(t, "GET example.com/", def.Pattern())
		}
		require.ErrorContains(t, muxt.CheckForDuplicatePatterns(definitions), `duplicate route pattern "GET example.com/"`, "it should find the duplicate")
	})

	t.Run("ensure paths are normalized", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "  /abc"}}a{{end}} {{define "/abc  "}}b{{end}}`))
		definitions, err := muxt.Definitions(ts, "ts")
		require.NoError(t, err)
		require.Len(t, definitions, 2)
		for _, def := range definitions {
			np := def.Pattern()
			rawPat := def.RawPattern()
			assert.Equalf(t, "/abc", np, "expected normalized pattern (raw %q, normalized %q)", rawPat, np)
		}
		require.ErrorContains(t, muxt.CheckForDuplicatePatterns(definitions), `duplicate route pattern "/abc"`, "it should find the duplicate")
	})
}
