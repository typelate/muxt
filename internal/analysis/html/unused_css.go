package analysis

import (
	"cmp"
	"fmt"
	"go/token"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/selectors"
)

// Stylesheet is CSS from a file the templates use, such as an embedded .css
// file.
type Stylesheet struct {
	Filename string
	Text     string
}

// StyleDiagnostic is a finding located in a template or a stylesheet file.
type StyleDiagnostic struct {
	Position token.Position // Offset is a byte offset; Line and Column are 1-based
	Severity Severity
	Code     template.DiagnosticCode
	Message  string
}

// CheckUnusedCSS reports the selectors of the <style> elements in trees and
// of sheets that no element in trees can match.
//
// The check is conservative. Selectors built with template actions, selectors
// the selectors package cannot compile, and at-rules other than the grouping
// rules (@media, @supports, @layer, @container, @starting-style) are not
// checked. Elements outside a lexical <html> element are reached through
// {{template}} calls, so their ancestors are unknown: a selector whose subject
// matches such an element counts as used. Selectors with sibling combinators
// or :has() count as used whenever their subject matches, since siblings and
// descendants may come from other templates. Classes added by scripts are not
// seen.
func CheckUnusedCSS(trees []*template.Tree, sheets []Stylesheet) []StyleDiagnostic {
	roots := templateRoots(trees)
	var diags []StyleDiagnostic
	for _, block := range styleBlocks(trees, sheets) {
		for _, d := range unusedSelectors(block.css, roots) {
			diags = append(diags, block.source.locate(d))
		}
	}
	return diags
}

// templateRoots returns the roots of trees and of the define and block bodies
// in them: selector queries do not descend into define and block bodies.
func templateRoots(trees []*template.Tree) []template.Node {
	var roots []template.Node
	seen := make(map[template.Node]bool)
	add := func(n template.Node) {
		if n == nil || seen[n] {
			return
		}
		seen[n] = true
		roots = append(roots, n)
	}
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		add(t.Root)
		template.Walk(t, func(_ *template.Tree, n template.Node) {
			switch n := n.(type) {
			case *template.DefineNode:
				if n.Body != nil {
					add(n.Body)
				}
			case *template.BlockNode:
				if n.Body != nil {
					add(n.Body)
				}
			}
		})
	}
	return roots
}

func unusedSelectors(css cssText, roots []template.Node) Diagnostics {
	sheet, ok := parseStylesheet(css)
	if !ok {
		return nil
	}
	var findings Diagnostics
	checkRules(css, sheet.Children, "", roots, &findings)
	return findings
}

// parseStylesheet parses css as a stylesheet with locations.
func parseStylesheet(css cssText) (*csstree.StyleSheet, bool) {
	ast, err := csstree.Parse(css.text, csstree.ParseOptions{Positions: true})
	if err != nil {
		return nil, false
	}
	sheet, ok := ast.(*csstree.StyleSheet)
	return sheet, ok
}

// groupingAtRules are the at-rules whose blocks hold style rules that apply
// as if written outside the at-rule.
var groupingAtRules = map[string]bool{
	"media":          true,
	"supports":       true,
	"layer":          true,
	"container":      true,
	"starting-style": true,
	"document":       true,
}

// checkRules checks the style rules in list. parent is the selector list the
// rules are nested in, already resolved, or "" at the top level.
func checkRules(css cssText, list *csstree.List[csstree.Node], parent string, roots []template.Node, findings *Diagnostics) {
	for item := list.Head(); item != nil; item = item.Next {
		switch n := item.Data.(type) {
		case *csstree.Atrule:
			if n.Block != nil && groupingAtRules[strings.ToLower(n.Name)] {
				checkRules(css, n.Block.Children, parent, roots, findings)
			}
		case *csstree.Rule:
			checkRule(css, n, parent, roots, findings)
		}
	}
}

func checkRule(css cssText, rule *csstree.Rule, parent string, roots []template.Node, findings *Diagnostics) {
	list, ok := rule.Prelude.(*csstree.SelectorList)
	if !ok || list.Loc == nil || css.dynamic(list.Loc.Start.Offset, preludeEnd(rule)) {
		return
	}
	var (
		resolved []string
		unused   []*csstree.Selector
		count    int
	)
	for item := list.Children.Head(); item != nil; item = item.Next {
		sel, ok := item.Data.(*csstree.Selector)
		if !ok || sel.Loc == nil {
			return
		}
		count++
		query, subject, ok := resolveSelector(sel, parent)
		if !ok {
			return
		}
		resolved = append(resolved, query)
		if selectorUse(query, subject, roots) == selectorUnused {
			unused = append(unused, sel)
		}
	}
	if len(unused) == count {
		*findings = append(*findings, Diagnostic{
			Pos:      css.pos(list.Loc.Start.Offset),
			End:      css.pos(list.Loc.End.Offset),
			Severity: SeverityWarning,
			Code:     template.DiagUnusedCSSRule,
			Message:  fmt.Sprintf(msgUnusedCSSRule, css.source(list.Loc)),
		})
	} else {
		for _, sel := range unused {
			*findings = append(*findings, Diagnostic{
				Pos:      css.pos(sel.Loc.Start.Offset),
				End:      css.pos(sel.Loc.End.Offset),
				Severity: SeverityWarning,
				Code:     template.DiagUnusedCSSSelector,
				Message:  fmt.Sprintf(msgUnusedCSSSelector, css.source(sel.Loc)),
			})
		}
	}
	if rule.Block != nil {
		checkRules(css, rule.Block.Children, strings.Join(resolved, ", "), roots, findings)
	}
}

// preludeEnd returns the offset of the block of rule. A selector list's
// location ends at its last token, so a template action blanked to a comment
// at the end of the selector lies between the location's end and the block.
func preludeEnd(rule *csstree.Rule) int {
	if rule.Block != nil && rule.Block.Loc != nil {
		return rule.Block.Loc.Start.Offset
	}
	return rule.Prelude.Location().End.Offset
}

// resolveSelector returns sel as a query for the selectors package, and the
// query for its subject (rightmost) compound selector. Pseudo-elements are
// dropped: they style a part of the element the rest of the selector
// matches. A nesting selector (&) becomes :is(parent); a nested selector
// without one is relative to parent as a descendant.
func resolveSelector(sel *csstree.Selector, parent string) (query, subject string, ok bool) {
	var q, s strings.Builder
	nested := false
	for item := sel.Children.Head(); item != nil; item = item.Next {
		var text string
		switch c := item.Data.(type) {
		case *csstree.PseudoElementSelector:
			continue
		case *csstree.NestingSelector:
			if parent == "" {
				return "", "", false
			}
			nested = true
			text = ":is(" + parent + ")"
		case *csstree.Combinator:
			s.Reset()
			generated, err := csstree.Generate(c, csstree.GenerateOptions{})
			if err != nil {
				return "", "", false
			}
			q.WriteString(generated)
			continue
		default:
			generated, err := csstree.Generate(c, csstree.GenerateOptions{})
			if err != nil {
				return "", "", false
			}
			text = generated
		}
		q.WriteString(text)
		s.WriteString(text)
	}
	query, subject = q.String(), cmp.Or(s.String(), "*")
	if strings.TrimSpace(query) == "" || strings.HasSuffix(query, " ") {
		query += "*"
	}
	if parent != "" && !nested {
		first := sel.Children.Head()
		if _, leadingCombinator := first.Data.(*csstree.Combinator); leadingCombinator {
			query = ":is(" + parent + ")" + query
		} else {
			query = ":is(" + parent + ") " + query
		}
	}
	return query, subject, true
}

type selectorUseResult int

const (
	selectorUsed selectorUseResult = iota
	selectorUnused
	selectorUnknown
)

// selectorUse reports whether an element in roots can match query, falling
// back to subject when the elements query relates the subject to may come
// from other templates.
func selectorUse(query, subject string, roots []template.Node) selectorUseResult {
	sel, err := selectors.Compile(query)
	if err != nil {
		return selectorUnknown
	}
	if anyMatch(sel, roots, func(*template.HTMLElementNode) bool { return true }) {
		return selectorUsed
	}
	if subject == query {
		return selectorUnused
	}
	subjectSel, err := selectors.Compile(subject)
	if err != nil {
		return selectorUnknown
	}
	outsideTemplate := strings.ContainsAny(query, "+~") || strings.Contains(strings.ToLower(query), ":has(")
	if anyMatch(subjectSel, roots, func(el *template.HTMLElementNode) bool { return outsideTemplate || !inDocument(el) }) {
		return selectorUsed
	}
	return selectorUnused
}

// anyMatch reports whether sel may match an element in roots for which keep
// returns true.
func anyMatch(sel selectors.Selector, roots []template.Node, keep func(*template.HTMLElementNode) bool) bool {
	for _, root := range roots {
		for el := range sel.QuerySelectorAll(root) {
			if keep(el) {
				return true
			}
		}
	}
	return false
}

// inDocument reports whether el has a lexical <html> ancestor, so all of its
// ancestors are in the same template.
func inDocument(el *template.HTMLElementNode) bool {
	type parented interface{ Parent() template.Node }
	for n := template.Node(el); n != nil; {
		if e, ok := n.(*template.HTMLElementNode); ok && strings.EqualFold(e.TagName, "html") {
			return true
		}
		p, ok := n.(parented)
		if !ok {
			return false
		}
		n = p.Parent()
	}
	return false
}
