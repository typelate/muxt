package cli

import (
	"os"
	"path/filepath"
	"slices"

	htmlanalysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/template"
)

// formatStyles is the author CSS muxt fmt classifies files with: the
// <style> elements of files and context, and the stylesheets they link to
// or import through the static assets routes the generated files in wd
// record, read from disk. Without a loaded package no route is known, so
// any stylesheet link is missing, and the author CSS is not known to be
// complete.
func formatStyles(wd string, packageLoaded bool, files, context []*template.Tree) (htmlanalysis.WhitespaceOptions, error) {
	var assets []htmlanalysis.StaticAssets
	if packageLoaded {
		configs, err := recordedStaticAssets(wd)
		if err != nil {
			return htmlanalysis.WhitespaceOptions{}, err
		}
		for _, config := range configs {
			prefix := generate.StaticAssetsPrefix(config.StaticAssetsPath)
			assets = append(assets, htmlanalysis.StaticAssets{
				Prefix:     prefix,
				PathMethod: muxt.StaticAssetsPathIdentifier(prefix),
				Files:      os.DirFS(filepath.Join(wd, filepath.FromSlash(config.StaticAssetsDirectory))),
			})
		}
	}
	sheets, missing, err := htmlanalysis.ResolveStylesheets(slices.Concat(files, context), assets)
	if err != nil {
		return htmlanalysis.WhitespaceOptions{}, err
	}
	return htmlanalysis.WhitespaceOptions{
		Context:        context,
		Sheets:         sheets,
		AuthorCSSKnown: packageLoaded,
		MissingSheets:  missing,
	}, nil
}
