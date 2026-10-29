package analysis

import (
	"fmt"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template"
)

// CheckCustomProperties reports var() references without a fallback to
// custom properties that no CSS in trees or sheets declares: no declaration
// in a style attribute, <style> element, or sheet, and no @property rule.
//
// Custom properties set by scripts or in stylesheets htt is not given are
// not seen. When a template may build a property name (a template action
// directly before a colon), any custom property may be declared, so nothing
// is reported.
func CheckCustomProperties(trees []*template.Tree, sheets []Stylesheet) []StyleDiagnostic {
	type reference struct {
		block styleBlock
		name  string
		loc   *csstree.Location
	}
	var (
		references []reference
		declared   = make(map[string]bool)
	)
	for _, block := range append(styleBlocks(trees, sheets), styleAttributes(trees)...) {
		if block.css.mayBuildPropertyName() {
			return nil
		}
		ast, ok := parseCustomPropertyValues(block)
		if !ok {
			continue
		}
		err := csstree.Walk(ast, csstree.WalkOptions{
			Enter: func(_ *csstree.WalkContext, node csstree.Node, _ *csstree.ListItem[csstree.Node], _ *csstree.List[csstree.Node]) csstree.WalkAction {
				switch n := node.(type) {
				case *csstree.Declaration:
					if strings.HasPrefix(n.Property, "--") {
						declared[n.Property] = true
					}
				case *csstree.Atrule:
					if strings.EqualFold(n.Name, "property") && n.Prelude != nil {
						if name, err := csstree.Generate(n.Prelude, csstree.GenerateOptions{}); err == nil {
							declared[strings.TrimSpace(name)] = true
						}
					}
				case *csstree.Function:
					name, ident, ok := varReference(n)
					if ok && ident.Loc != nil && !block.css.dynamic(n.Loc.Start.Offset, n.Loc.End.Offset) {
						references = append(references, reference{block: block, name: name, loc: ident.Loc})
					}
				}
				return csstree.WalkContinue
			},
		})
		if err != nil {
			continue
		}
	}
	var diags []StyleDiagnostic
	for _, r := range references {
		if declared[r.name] {
			continue
		}
		diags = append(diags, r.block.source.locate(Diagnostic{
			Pos:      r.block.css.pos(r.loc.Start.Offset),
			End:      r.block.css.pos(r.loc.End.Offset),
			Severity: SeverityWarning,
			Code:     template.DiagUndefinedCSSCustomProperty,
			Message:  fmt.Sprintf(msgUndefinedCSSCustomProperty, r.name),
		}))
	}
	return sortStyleDiagnostics(diags)
}

// parseCustomPropertyValues parses block with custom property values parsed
// as values, so var() references in them are function nodes.
func parseCustomPropertyValues(block styleBlock) (csstree.Node, bool) {
	options := csstree.ParseOptions{Positions: true, ParseCustomProperty: true}
	if block.attribute {
		options.Context = "declarationList"
	}
	ast, err := csstree.Parse(block.css.text, options)
	return ast, err == nil
}

// varReference returns the custom property a var() function refers to, when
// it has no fallback value.
func varReference(fn *csstree.Function) (string, *csstree.Identifier, bool) {
	if !strings.EqualFold(fn.Name, "var") || fn.Children == nil {
		return "", nil, false
	}
	var ident *csstree.Identifier
	for item := fn.Children.Head(); item != nil; item = item.Next {
		switch n := item.Data.(type) {
		case *csstree.WhiteSpace:
		case *csstree.Identifier:
			if ident != nil || !strings.HasPrefix(n.Name, "--") {
				return "", nil, false
			}
			ident = n
		default:
			return "", nil, false // a fallback, or not a custom property name
		}
	}
	if ident == nil {
		return "", nil, false
	}
	return ident.Name, ident, true
}

// mayBuildPropertyName reports whether a template action comes right before
// a colon, where it may write a property name.
func (c cssText) mayBuildPropertyName() bool {
	for _, g := range c.gaps {
		rest := strings.TrimLeft(c.text[g.end:], " \t\r\n\f")
		if strings.HasPrefix(rest, ":") {
			return true
		}
	}
	return false
}
