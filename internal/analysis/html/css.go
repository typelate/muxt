package analysis

import (
	"cmp"
	"slices"

	"github.com/typelate/muxt/internal/template"
)

// CheckCSS runs every CSS check over the templates trees and the
// stylesheets they use: declarations browsers drop (in style attributes,
// <style> elements, and sheets), unknown at-rules, CSS no template element
// can match, and var() references to custom properties nothing declares. A template reached through several trees is reported
// once. The result is sorted by file, then offset, then code.
func CheckCSS(trees []*template.Tree, sheets []Stylesheet) []StyleDiagnostic {
	var diags []StyleDiagnostic
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		source := templateSource(t)
		for _, d := range CheckStyleAttributes(t) {
			diags = append(diags, source.locate(d))
		}
	}
	for _, block := range styleBlocks(trees, sheets) {
		for _, d := range checkStyleBlock(block.css) {
			diags = append(diags, block.source.locate(d))
		}
	}
	diags = append(diags, CheckUnusedCSS(trees, sheets)...)
	diags = append(diags, CheckCustomProperties(trees, sheets)...)
	return sortStyleDiagnostics(diags)
}

// sortStyleDiagnostics removes duplicates from diags and sorts it by file,
// then offset, then code.
func sortStyleDiagnostics(diags []StyleDiagnostic) []StyleDiagnostic {
	seen := make(map[StyleDiagnostic]bool)
	unique := diags[:0]
	for _, d := range diags {
		if seen[d] {
			continue
		}
		seen[d] = true
		unique = append(unique, d)
	}
	slices.SortFunc(unique, func(a, b StyleDiagnostic) int {
		return cmp.Or(
			cmp.Compare(a.Position.Filename, b.Position.Filename),
			cmp.Compare(a.Position.Offset, b.Position.Offset),
			cmp.Compare(a.Code, b.Code),
		)
	})
	return unique
}
