package analysis

import "github.com/typelate/htt"

// The codes of the CSS findings this package reports.
const (
	DiagUnknownCSSProperty htt.DiagnosticCode = "unknown-css-property"
	DiagInvalidCSSValue    htt.DiagnosticCode = "invalid-css-value"
	DiagUnknownCSSAtRule   htt.DiagnosticCode = "unknown-css-at-rule"

	DiagUndefinedCSSCustomProperty htt.DiagnosticCode = "undefined-css-custom-property"
	DiagUnusedCSSRule              htt.DiagnosticCode = "unused-css-rule"
	DiagUnusedCSSSelector          htt.DiagnosticCode = "unused-css-selector"
)
