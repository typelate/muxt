package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	htmlanalysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/header"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestFormatStyles(t *testing.T) {
	wd := t.TempDir()
	writeTestFile(t, wd, "go.mod", "module example.com\n\ngo 1.25\n")
	writeTestFile(t, wd, "templates.go", "package main\n\nimport (\n\t\"embed\"\n\t\"html/template\"\n)\n\n//go:embed *.gohtml\nvar templateSource embed.FS\n\nvar templates = template.Must(template.ParseFS(templateSource, \"*\"))\n")
	require.NoError(t, os.Mkdir(filepath.Join(wd, "assets"), 0o755))
	writeTestFile(t, wd, filepath.Join("assets", "app.css"), ".code{white-space:pre}")
	writeTestFile(t, wd, "template_routes.go", header.Format([]string{"--input-static-assets-directory=assets", "--output-static-assets-path=/static"}, "v1")+"package main\n")

	parse := func(source string) []*template.Tree {
		t.Helper()
		tree, err := template.ParseTree("t.gohtml", "t.gohtml", source, nil, lex.NewOptions())
		require.NoError(t, err, "ParseTree(%q)", source)
		return []*template.Tree{tree}
	}
	const linked = `<link rel="stylesheet" href="/static/app.css"><div class="code"></div>`
	appCSS := []htmlanalysis.Stylesheet{{Filename: "/static/app.css", Text: ".code{white-space:pre}"}}

	t.Run("a linked sheet", func(t *testing.T) {
		files := parse(linked)
		got, err := formatStyles(wd, true, files, nil)
		require.NoError(t, err, "formatStyles()")
		assert.Equal(t, htmlanalysis.WhitespaceOptions{Sheets: appCSS, AuthorCSSKnown: true}, got, "formatStyles(%q)", linked)
	})
	t.Run("a missing sheet", func(t *testing.T) {
		const source = `<link rel="stylesheet" href="/static/gone.css">`
		got, err := formatStyles(wd, true, parse(source), nil)
		require.NoError(t, err, "formatStyles()")
		assert.Equal(t, htmlanalysis.WhitespaceOptions{AuthorCSSKnown: true, MissingSheets: true}, got, "formatStyles(%q)", source)
	})
	t.Run("no package", func(t *testing.T) {
		got, err := formatStyles(wd, false, parse(linked), nil)
		require.NoError(t, err, "formatStyles()")
		assert.Equal(t, htmlanalysis.WhitespaceOptions{MissingSheets: true}, got, "formatStyles(%q) without a package", linked)
	})
	t.Run("a link in a context tree", func(t *testing.T) {
		files, context := parse(`<div class="code"></div>`), parse(linked)
		got, err := formatStyles(wd, true, files, context)
		require.NoError(t, err, "formatStyles()")
		assert.Equal(t, htmlanalysis.WhitespaceOptions{Context: context, Sheets: appCSS, AuthorCSSKnown: true}, got, "formatStyles(%q) with context %q", `<div class="code"></div>`, linked)
	})
}
