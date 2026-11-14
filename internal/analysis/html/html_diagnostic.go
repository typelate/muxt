package analysis

import (
	"fmt"
	"strings"

	"github.com/typelate/htt"
)

type Severity = htt.Severity

const (
	SeverityWarning = htt.SeverityWarning
	SeverityError   = htt.SeverityError
)

type (
	Diagnostic  = htt.Diagnostic
	Diagnostics = htt.Diagnostics
)

// CheckStructure walks the parsed HTML AST and reports structural issues
// such as unclosed elements. It returns all diagnostics found.
func CheckStructure(t *htt.Tree) Diagnostics {
	if t == nil || t.Root == nil {
		return nil
	}
	diags := append(Diagnostics(nil), t.Diagnostics...)
	htt.Walk(t, func(_ *htt.Tree, n htt.Node) {
		elem, ok := n.(*htt.HTMLElementNode)
		if !ok || elem == nil || elem.Void || elem.Closed ||
			htt.ImpliedEndTagElements[strings.ToLower(elem.TagName)] ||
			elem.Misnested {
			return
		}
		diags = append(diags, Diagnostic{
			Pos:      elem.Start(),
			Severity: SeverityWarning,
			Code:     htt.DiagUnclosedElement,
			Message:  fmt.Sprintf("element <%s> is not closed", elem.TagName),
		})
	})
	checkFosterParenting(t, &diags)
	checkNesting(t, &diags)
	checkAutoClose(t, &diags)
	diags.SortByPos()
	return diags
}
