package analysis

import (
	"fmt"
	"strings"

	"github.com/typelate/muxt/internal/template"
)

type Severity = template.Severity

const (
	SeverityWarning = template.SeverityWarning
	SeverityError   = template.SeverityError
)

type (
	Diagnostic  = template.Diagnostic
	Diagnostics = template.Diagnostics
)

// CheckStructure walks the parsed HTML AST and reports structural issues
// such as unclosed elements. It returns all diagnostics found.
func CheckStructure(t *template.Tree) Diagnostics {
	if t == nil || t.Root == nil {
		return nil
	}
	diags := append(Diagnostics(nil), t.Diagnostics...)
	template.Walk(t, func(_ *template.Tree, n template.Node) {
		elem, ok := n.(*template.HTMLElementNode)
		if !ok || elem == nil || elem.Void || elem.Closed ||
			template.ImpliedEndTagElements[strings.ToLower(elem.TagName)] ||
			elem.Misnested {
			return
		}
		diags = append(diags, Diagnostic{
			Pos:      elem.Start(),
			Severity: SeverityWarning,
			Code:     template.DiagUnclosedElement,
			Message:  fmt.Sprintf("element <%s> is not closed", elem.TagName),
		})
	})
	checkFosterParenting(t, &diags)
	checkNesting(t, &diags)
	checkAutoClose(t, &diags)
	diags.SortByPos()
	return diags
}
