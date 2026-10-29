package analysis

import (
	"fmt"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template"
)

// CheckStyleElements reports what browsers drop in the <style> elements of
// t: declarations with unknown properties or invalid values, and unknown
// at-rules. Declarations in descriptor blocks such as @font-face and @page
// are not checked.
func CheckStyleElements(t *template.Tree) Diagnostics {
	var diags Diagnostics
	for _, block := range styleBlocks([]*template.Tree{t}, nil) {
		diags = append(diags, checkStyleBlock(block.css)...)
	}
	diags.SortByPos()
	return diags
}

// CheckStylesheet reports what browsers drop in sheet, like
// CheckStyleElements.
func CheckStylesheet(sheet Stylesheet) []StyleDiagnostic {
	var diags []StyleDiagnostic
	for _, block := range styleBlocks(nil, []Stylesheet{sheet}) {
		for _, d := range checkStyleBlock(block.css) {
			diags = append(diags, block.source.locate(d))
		}
	}
	return diags
}

func checkStyleBlock(css cssText) Diagnostics {
	sheet, ok := parseStylesheet(css)
	if !ok {
		return nil
	}
	var diags Diagnostics
	checkStatements(css, sheet.Children, len(css.text), false, &diags)
	diags.SortByPos()
	return diags
}

// checkStatements checks the rules and at-rules in list, which ends at end.
// inStyleRule reports that list is the block of a style rule, or of a
// grouping at-rule nested in one, so its declarations are properties.
func checkStatements(css cssText, list *csstree.List[csstree.Node], end int, inStyleRule bool, diags *Diagnostics) {
	if inStyleRule {
		*diags = append(*diags, checkDeclarationList(css, list, end)...)
	}
	for item := list.Head(); item != nil; item = item.Next {
		switch n := item.Data.(type) {
		case *csstree.Rule:
			if n.Block != nil {
				checkStatements(css, n.Block.Children, blockEnd(css, n.Block), true, diags)
			}
		case *csstree.Atrule:
			checkAtruleName(css, n, diags)
			if n.Block != nil && groupingAtRules[strings.ToLower(n.Name)] {
				checkStatements(css, n.Block.Children, blockEnd(css, n.Block), inStyleRule, diags)
			}
		}
	}
}

// blockEnd returns the offset of the closing brace of block.
func blockEnd(css cssText, block *csstree.Block) int {
	if block.Loc == nil || block.Loc.End.Offset == 0 {
		return len(css.text)
	}
	return block.Loc.End.Offset - len("}")
}

// checkAtruleName reports an at-rule browsers do not know.
func checkAtruleName(css cssText, rule *csstree.Atrule, diags *Diagnostics) {
	if rule.Loc == nil || strings.HasPrefix(rule.Name, "-") {
		return
	}
	start := rule.Loc.Start.Offset
	end := start + len("@") + len(rule.Name)
	if css.dynamic(start, end) || csstree.DefaultLexer().CheckAtruleName(rule.Name) == nil {
		return
	}
	*diags = append(*diags, Diagnostic{
		Pos:      css.pos(start),
		End:      css.pos(end),
		Severity: SeverityWarning,
		Code:     template.DiagUnknownCSSAtRule,
		Message:  fmt.Sprintf(msgUnknownCSSAtRule, "@"+rule.Name),
	})
}
