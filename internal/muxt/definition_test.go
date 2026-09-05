package muxt_test

import (
	"errors"
	"html/template"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/muxt"
)

func TestDefinitions(t *testing.T) {
	t.Run("when one of the template names is a malformed pattern", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "HEAD /"}}{{end}}`))
		_, err := muxt.Definitions(ts, "ts", nil)
		require.Error(t, err)
	})
}

func TestCheckPathMethodCollisions(t *testing.T) {
	t.Run("when two handlers differ only in the case of the first letter", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET /items list(ctx)"}}{{end}}{{define "GET /items/{id} List(ctx, id)"}}{{end}}`))
		defs, err := muxt.Definitions(ts, "ts", nil)
		require.NoError(t, err)
		require.ErrorContains(t, muxt.CheckPathMethodCollisions(defs), `TemplateRoutePaths method name collision: handlers "list" and "List" both produce method "List"`)
	})
	t.Run("when a handler identifier cannot be exported", func(t *testing.T) {
		// 一覧 (Japanese "list") has no uppercase form, so no exported
		// TemplateRoutePaths method name can be derived from it.
		ts := template.Must(template.New("").Parse(`{{define "GET /items 一覧(ctx)"}}{{end}}`))
		defs, err := muxt.Definitions(ts, "ts", nil)
		require.NoError(t, err)
		require.ErrorContains(t, muxt.CheckPathMethodCollisions(defs), `cannot export identifier "一覧" for TemplateRoutePaths method: first character '一' has no uppercase form`)
	})
	t.Run("when handlers produce distinct method names", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET /items List(ctx)"}}{{end}}{{define "GET /items/{id} Show(ctx, id)"}}{{end}}`))
		defs, err := muxt.Definitions(ts, "ts", nil)
		require.NoError(t, err)
		require.NoError(t, muxt.CheckPathMethodCollisions(defs))
	})
}

func TestCheckForDuplicatePatterns(t *testing.T) {
	t.Run("when the pattern is not unique", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET  / F1()"}}a{{end}} {{define "GET /  F2()"}}b{{end}}`))
		definitions, err := muxt.Definitions(ts, "ts", nil)
		require.NoError(t, err)
		require.Len(t, definitions, 2)
		for _, def := range definitions {
			assert.Equal(t, "GET /", def.Pattern())
		}
		require.ErrorContains(t, muxt.CheckForDuplicatePatterns(definitions), `duplicate route pattern "GET /"`, "it should find the duplicate")
	})

	t.Run("ensure hosts are normalized", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET  example.com/ F1()"}}a{{end}} {{define "GET Example.COM/  F2()"}}b{{end}}`))
		definitions, err := muxt.Definitions(ts, "ts", nil)
		require.NoError(t, err)
		require.Len(t, definitions, 2)
		for _, def := range definitions {
			assert.Equal(t, "GET example.com/", def.Pattern())
		}
		require.ErrorContains(t, muxt.CheckForDuplicatePatterns(definitions), `duplicate route pattern "GET example.com/"`, "it should find the duplicate")
	})

	t.Run("ensure paths are normalized", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "  /abc"}}a{{end}} {{define "/abc  "}}b{{end}}`))
		definitions, err := muxt.Definitions(ts, "ts", nil)
		require.NoError(t, err)
		require.Len(t, definitions, 2)
		for _, def := range definitions {
			np := def.Pattern()
			rawPat := def.RawPattern()
			assert.Equalf(t, "/abc", np, "expected normalized pattern (raw %q, normalized %q)", rawPat, np)
		}
		require.ErrorContains(t, muxt.CheckForDuplicatePatterns(definitions), `duplicate route pattern "/abc"`, "it should find the duplicate")
	})

	t.Run("the short form is one line and the long form has one location per line", func(t *testing.T) {
		dupErr := &muxt.DuplicatePatternError{
			Pattern:   "GET /",
			Locations: []string{"index.gohtml:1:11", "index.gohtml:5:11", "other.gohtml:2:11"},
		}
		require.Equal(t, `duplicate route pattern "GET /"`, dupErr.Error())
		var sb strings.Builder
		require.NoError(t, dupErr.DetailedError(&sb))
		require.Equal(t, "index.gohtml:1:11: first defined here\nindex.gohtml:5:11: also defined here\nother.gohtml:2:11: also defined here\n", sb.String())
	})

	t.Run("unknown locations leave the long form empty", func(t *testing.T) {
		dupErr := &muxt.DuplicatePatternError{Pattern: "GET /"}
		var sb strings.Builder
		require.NoError(t, dupErr.DetailedError(&sb))
		require.Empty(t, sb.String())
	})

	t.Run("all definitions of the pattern are reported in a stable order", func(t *testing.T) {
		// Three different handler calls across two files, one normalized
		// pattern. The template set iterates in map order, so the error
		// must sort: by file, then by position, then by name.
		fsys := fstest.MapFS{
			"b.gohtml": &fstest.MapFile{Data: []byte(`{{define "GET /  F2()"}}b{{end}}{{define "GET / F3()"}}c{{end}}`)},
			"a.gohtml": &fstest.MapFile{Data: []byte(`{{define "GET  / F1()"}}a{{end}}`)},
		}
		ts := template.Must(template.ParseFS(fsys, "*.gohtml"))
		definitions, err := muxt.Definitions(ts, "ts", nil)
		require.NoError(t, err)
		require.Len(t, definitions, 3)
		for range 8 {
			err := muxt.CheckForDuplicatePatterns(definitions)
			dupErr, ok := errors.AsType[*muxt.DuplicatePatternError](err)
			require.True(t, ok)
			require.Equal(t, "GET /", dupErr.Pattern)
			require.Equal(t, []string{"a.gohtml", "b.gohtml", "b.gohtml"}, dupErr.Locations)
		}
	})
}
