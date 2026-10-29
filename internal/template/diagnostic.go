package template

import (
	"fmt"
	"slices"

	"github.com/typelate/muxt/internal/template/lex"
)

// Severity indicates how a structural diagnostic should be treated.
type Severity int

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return fmt.Sprintf("Severity(%d)", int(s))
	}
}

// DiagnosticCode identifies a class of structural finding. Codes are
// stable identifiers for tests, suppression, and LSP mapping.
type DiagnosticCode string

const (
	DiagUnclosedElement DiagnosticCode = "unclosed-element"
	DiagOrphanEndTag    DiagnosticCode = "orphan-end-tag"
	DiagMisnestedTag    DiagnosticCode = "misnested-tag"    // adoption agency (WHATWG §13.2.6.4.7)
	DiagFosteredContent DiagnosticCode = "fostered-content" // foster parenting (§13.2.6.4.9)
	DiagNestedForm      DiagnosticCode = "nested-form"      // §13.2.6.4.7 form pointer
	DiagAutoClosed      DiagnosticCode = "auto-closed"      // implied end tags (§13.2.6.3)
	DiagNestedLink      DiagnosticCode = "nested-link"
	DiagNestedButton    DiagnosticCode = "nested-button"

	DiagUnknownCSSProperty DiagnosticCode = "unknown-css-property"
	DiagInvalidCSSValue    DiagnosticCode = "invalid-css-value"
	DiagUnknownCSSAtRule   DiagnosticCode = "unknown-css-at-rule"

	DiagUndefinedCSSCustomProperty DiagnosticCode = "undefined-css-custom-property"
	DiagUnusedCSSRule              DiagnosticCode = "unused-css-rule"
	DiagUnusedCSSSelector          DiagnosticCode = "unused-css-selector"
)

// Diagnostic is a non-fatal structural finding: the tree still
// faithfully represents the source, but a browser's tree construction
// would diverge from the source's literal nesting at this position.
type Diagnostic struct {
	Pos      lex.Pos
	End      lex.Pos
	Severity Severity
	Code     DiagnosticCode
	Message  string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s at pos %d: %s", d.Severity, d.Pos, d.Message)
}

type Diagnostics []Diagnostic

func (ds Diagnostics) SortByPos() {
	slices.SortStableFunc(ds, func(a, b Diagnostic) int { return int(a.Pos - b.Pos) })
}

// diag records a parse-time structural diagnostic on the tree.
func (t *Tree) diag(pos, end lex.Pos, sev Severity, code DiagnosticCode, format string, args ...any) {
	t.Diagnostics = append(t.Diagnostics, Diagnostic{
		Pos: pos, End: end, Severity: sev, Code: code,
		Message: fmt.Sprintf(format, args...),
	})
}

// Message formats. Centralized so the message-quality pass (plan Task
// 10) edits one place; analysis-side formats live in analysis/messages.go.
const (
	msgOrphanEndTag   = `unmatched </%s> is ignored by browsers`
	msgMisnestedClose = `</%s> also closes <%s>; browsers reopen a copy of <%s> after it`
)
