package analysis

import (
	"errors"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/typelate/muxt/internal/template"
)

func TestResolveStylesheets(t *testing.T) {
	file := func(text string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(text)} }
	assets := []StaticAssets{
		{Prefix: "/static/", PathMethod: "ReadStatic", Files: fstest.MapFS{
			"app.css":     file(`@import "parts/b.css";p{color:red}`),
			"parts/b.css": file(`@import "../app.css";`),
			"x.css":       file(`div{}`),
			"_draft.css":  file(`p{}`),
		}},
		{Prefix: "/static/x/", Files: fstest.MapFS{"y.css": file(``)}},
	}
	for _, test := range []struct {
		source  string
		want    []string
		missing bool
	}{
		{source: `<p>x</p>`},
		{source: `<link rel="stylesheet" href="/static/x.css">`, want: []string{"/static/x.css"}},
		{source: `<link rel="Stylesheet preload" href="static/x.css">`, want: []string{"/static/x.css"}},
		{source: `<link rel="stylesheet" href="/static/app.css">`, want: []string{"/static/app.css", "/static/parts/b.css"}},
		{source: `<link rel="stylesheet" href="{{.Path.ReadStatic}}x.css">`, want: []string{"/static/x.css"}},
		{source: `<link rel="stylesheet" href="{{$.Path.ReadStatic}}x.css">`, want: []string{"/static/x.css"}},
		{source: `<link rel="stylesheet" href="/static/x/y.css">`, want: []string{"/static/x/y.css"}},
		{source: `<link rel="icon" href="https://cdn.example/a.css">`},
		{source: `<link rel="stylesheet" href="https://cdn.example/a.css">`, missing: true},
		{source: `<link rel="stylesheet" href="//cdn.example/a.css">`, missing: true},
		{source: `<link rel="stylesheet" href="/other/a.css">`, missing: true},
		{source: `<link rel="stylesheet" href="/static/nope.css">`, missing: true},
		{source: `<link rel="stylesheet" href="/static/parts/">`, missing: true},
		{source: `<link rel="stylesheet" href="/static/_draft.css">`, missing: true},
		{source: `<link rel="stylesheet" href="{{.CSS}}">`, missing: true},
		{source: `<link rel="stylesheet" href="{{.Path.ReadStatic | printf "%s"}}x.css">`, missing: true},
		{source: `<link rel="{{.Rel}}" href="/static/x.css">`, missing: true},
		{source: `<style>@import "/static/x.css";</style>`, want: []string{"/static/x.css"}},
		{source: `<style>@import "/elsewhere.css";</style>`, missing: true},
		{source: `{{if .X}}<link rel="stylesheet" href="/static/x.css">{{end}}`, want: []string{"/static/x.css"}},
		{source: `{{define "head"}}<link rel="stylesheet" href="/static/x.css">{{end}}`, want: []string{"/static/x.css"}},
		{source: `<link {{if .X}}rel="stylesheet"{{end}} href="/static/x.css">`, missing: true},
		{source: `<link rel="stylesheet" href="/static/x.css?v=2#top">`, want: []string{"/static/x.css"}},
		{source: `<link rel=stylesheet href=/static/x.css>`, want: []string{"/static/x.css"}},
	} {
		t.Run(test.source, func(t *testing.T) {
			trees := []*template.Tree{parseWhitespaceTree(t, "t.gohtml", test.source)}
			sheets, missing, err := ResolveStylesheets(trees, assets)
			if err != nil {
				t.Fatalf("ResolveStylesheets(%q) error: %v", test.source, err)
			}
			var got []string
			for _, sheet := range sheets {
				got = append(got, sheet.Filename)
			}
			if !slices.Equal(got, test.want) || missing != test.missing {
				t.Errorf("ResolveStylesheets(%q) = %v, %v; want %v, %v", test.source, got, missing, test.want, test.missing)
			}
		})
	}
}

// permissionFS fails every Open with fs.ErrPermission.
type permissionFS struct{}

func (permissionFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}

func TestResolveStylesheetsReadError(t *testing.T) {
	const source = `<link rel="stylesheet" href="/static/x.css">`
	trees := []*template.Tree{parseWhitespaceTree(t, "t.gohtml", source)}
	_, _, err := ResolveStylesheets(trees, []StaticAssets{{Prefix: "/static/", Files: permissionFS{}}})
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("ResolveStylesheets(%q) error = %v, want fs.ErrPermission", source, err)
	}
}

// TestResolvedStylesheetsWhitespace classifies a span with class x, which
// blk.css makes block and pre.css preserves white-space in, when the sheet
// may not apply.
func TestResolvedStylesheetsWhitespace(t *testing.T) {
	file := func(text string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(text)} }
	assets := []StaticAssets{{Prefix: "/static/", PathMethod: "ReadStatic", Files: fstest.MapFS{
		"blk.css":      file(`.x{display:block}`),
		"pre.css":      file(`.x{white-space:pre}`),
		"imp.css":      file(`@import "blk.css";`),
		"print.css":    file(`@import "blk.css" print;`),
		"supports.css": file(`@import "blk.css" supports(display:grid);`),
		"all.css":      file(`@import "blk.css" all;`),
	}}}
	const (
		blk = `<link rel="stylesheet" href="/static/blk.css">`
		pre = `<link rel="stylesheet" href="/static/pre.css">`
		imp = `<link rel="stylesheet" href="/static/imp.css">`
	)
	block := Whitespace{DisplayBlock, WhiteSpaceCollapse}
	for _, test := range []struct {
		source string
		want   Whitespace
	}{
		{source: blk + `<span class="x"></span>`, want: block},
		{source: pre + `<span class="x"></span>`, want: Whitespace{DisplayInline, WhiteSpacePreserve}},
		{source: imp + `<span class="x"></span>`, want: block},
		{source: `{{if .X}}` + blk + `{{end}}<span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `{{if .X}}` + pre + `{{end}}<span class="x"></span>`, want: Whitespace{DisplayInline, WhiteSpaceUnknown}},
		{source: `{{range .}}` + blk + `{{end}}<span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `{{with .X}}` + blk + `{{end}}<span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `{{if .X}}` + imp + `{{end}}<span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `{{if .X}}` + blk + `{{end}}` + blk + `<span class="x"></span>`, want: block},
		{source: blk + `{{if .X}}` + blk + `{{end}}<span class="x"></span>`, want: block},
		{source: `{{if .X}}` + imp + `{{end}}` + blk + `<span class="x"></span>`, want: block},
		{source: `{{if .X}}` + blk + `{{end}}` + imp + `<span class="x"></span>`, want: block},
		{source: `<link rel="stylesheet" media="print" href="/static/blk.css"><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<link rel="stylesheet" media="{{.M}}" href="/static/blk.css"><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<link rel="stylesheet" media="all" href="/static/blk.css"><span class="x"></span>`, want: block},
		{source: `<link rel="stylesheet" media="" href="/static/blk.css"><span class="x"></span>`, want: block},
		{source: `<link rel="alternate stylesheet" href="/static/blk.css"><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<link rel="stylesheet" disabled href="/static/blk.css"><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<template>` + blk + `</template><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<noscript>` + blk + `</noscript><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<link rel="stylesheet" href="/static/print.css"><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<link rel="stylesheet" href="/static/supports.css"><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `<link rel="stylesheet" href="/static/all.css"><span class="x"></span>`, want: block},
		{source: `<style>@import "/static/blk.css" print;</style><span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
		{source: `{{if .X}}<style>@import "/static/blk.css";</style>{{end}}<span class="x"></span>`, want: Whitespace{DisplayUnknown, WhiteSpaceCollapse}},
	} {
		t.Run(test.source, func(t *testing.T) {
			trees := []*template.Tree{parseWhitespaceTree(t, "t.gohtml", test.source)}
			sheets, missing, err := ResolveStylesheets(trees, assets)
			if err != nil || missing {
				t.Fatalf("ResolveStylesheets(%q) = _, %v, %v; want no missing sheet and no error", test.source, missing, err)
			}
			el := queryElement(t, trees, `span`)
			got := ComputeWhitespace(trees, WhitespaceOptions{Sheets: sheets, AuthorCSSKnown: true})[el]
			if got != test.want {
				t.Errorf("ComputeWhitespace(%q) with its resolved sheets [span] = %+v, want %+v", test.source, got, test.want)
			}
		})
	}
}
