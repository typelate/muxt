package analysis

import (
	"fmt"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// WHATWG §13.2.6.4.9: content allowed inside table-context elements
// without being foster-parented (§13.2.6.1) out to before the table.
var fosterParents = map[string]bool{"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true}

var tableContentAllowed = map[string]bool{
	"caption": true, "colgroup": true, "col": true, "thead": true,
	"tbody": true, "tfoot": true, "tr": true, "td": true, "th": true,
	"style": true, "script": true, "template": true, "form": true,
}

// checkFosterParenting reports content nested directly inside a
// table/thead/tbody/tfoot/tr that browsers foster-parent out to
// before the <table> during tree construction, rather than keeping
// where the source places it.
func checkFosterParenting(t *template.Tree, diags *Diagnostics) {
	template.WalkPath(t, func(_ *template.Tree, path []template.Node, n template.Node) {
		enc, mod := enclosingElement(path)
		if enc == nil || !fosterParents[strings.ToLower(enc.TagName)] {
			return
		}
		switch node := n.(type) {
		case *template.HTMLElementNode:
			tag := strings.ToLower(node.TagName)
			// input is allowed only when its type is the literal value
			// "hidden": browsers foster-parent any other <input> out of
			// the table, but a hidden input has no rendering, so authors
			// commonly place it inline for locality. A dynamic
			// type="{{.T}}" cannot be verified as "hidden" at check time,
			// so it is conservatively treated as NOT hidden and flagged
			// — dynamic type on a table-level input is rare enough that
			// warning is the more useful default.
			if tableContentAllowed[tag] || (tag == "input" && hasLiteralAttr(node, "type", "hidden")) {
				return
			}
			appendDiag(diags, node.Start(), node.End(), template.DiagFosteredContent, mod,
				fmt.Sprintf(msgFosteredElement, node.TagName, enc.TagName))
		case *template.TextNode:
			if node.Verbatim || !containsNonWhitespace(node.Text) {
				return
			}
			appendDiag(diags, node.Start(), node.End(), template.DiagFosteredContent, mod,
				fmt.Sprintf(msgFosteredText, enc.TagName))
		}
	})
}

// hasLiteralAttr reports whether el has an attribute named key (case
// insensitive) whose value is a single literal text node equal to
// value (case insensitive).
func hasLiteralAttr(el *template.HTMLElementNode, key, value string) bool {
	for _, a := range el.Attr {
		attr, ok := a.(*template.HTMLAttributeNode)
		if !ok || len(attr.Key) != 1 || len(attr.Value) != 1 {
			continue
		}
		k, kOK := attr.Key[0].(*template.TextNode)
		v, vOK := attr.Value[0].(*template.TextNode)
		if !kOK || !vOK {
			continue
		}
		if strings.EqualFold(string(k.Text), key) && strings.EqualFold(unquoteAttrValue(v.Text), value) {
			return true
		}
	}
	return false
}

// unquoteAttrValue strips a matching pair of surrounding single or
// double quotes from an attribute value's source text, if present.
// TextNode.Text for a quoted attribute value includes the quote
// characters verbatim (they are not part of the value itself).
func unquoteAttrValue(b []byte) string {
	if len(b) >= 2 {
		first, last := b[0], b[len(b)-1]
		if (first == '"' || first == '\'') && first == last {
			return string(b[1 : len(b)-1])
		}
	}
	return string(b)
}

// containsNonWhitespace reports whether b contains any byte outside
// the ASCII whitespace set (WHATWG: space, tab, LF, FF, CR).
func containsNonWhitespace(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\f', '\r':
		default:
			return true
		}
	}
	return false
}

// appendDiag appends a warning diagnostic, appending a modality
// suffix to msg when mod indicates the relationship does not hold on
// every render (see appendDiagSev).
func appendDiag(diags *Diagnostics, pos, end lex.Pos, code template.DiagnosticCode, mod Modality, msg string) {
	appendDiagSev(diags, pos, end, code, SeverityWarning, mod, msg)
}

// appendDiagSev appends a diagnostic at the given severity, appending
// a modality suffix to msg when mod indicates the relationship does
// not hold on every render: repeatedSuffix inside a {{range}} body,
// conditionalSuffix inside an {{if}}/{{with}} branch.
func appendDiagSev(diags *Diagnostics, pos, end lex.Pos, code template.DiagnosticCode, sev Severity, mod Modality, msg string) {
	switch mod {
	case ModalityRepeatedly:
		msg += repeatedSuffix
	case ModalityConditionally:
		msg += conditionalSuffix
	}
	*diags = append(*diags, Diagnostic{
		Pos: pos, End: end, Severity: sev, Code: code, Message: msg,
	})
}
