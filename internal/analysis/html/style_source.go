package analysis

import (
	"cmp"
	"go/token"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// styleSource is where CSS text came from: a template or a stylesheet file.
type styleSource struct {
	tree     *template.Tree // nil for a stylesheet file
	filename string
	text     string // the stylesheet file's text when tree is nil
}

// styleBlock is CSS from a <style> element, a stylesheet file, or a style
// attribute.
type styleBlock struct {
	source styleSource
	css    cssText
	// attribute reports that css is a style attribute's declaration list
	// rather than a stylesheet.
	attribute bool
}

// styleAttributes returns the style attributes of trees, each once even when
// several trees hold it.
func styleAttributes(trees []*template.Tree) []styleBlock {
	var blocks []styleBlock
	seen := make(map[token.Position]bool)
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		source := templateSource(t)
		template.Walk(t, func(_ *template.Tree, n template.Node) {
			attr, ok := n.(*template.HTMLAttributeNode)
			if !ok || len(attr.Value) == 0 || !strings.EqualFold(staticAttrName(attr), "style") {
				return
			}
			css := templateCSS(attr.Value)
			css.unquote()
			key := token.Position{Filename: source.filename, Offset: int(css.base)}
			if seen[key] {
				return
			}
			seen[key] = true
			blocks = append(blocks, styleBlock{source: source, css: css, attribute: true})
		})
	}
	return blocks
}

// locate places d, whose positions are template positions or byte offsets
// in a stylesheet file, at a file, line and column.
func (s styleSource) locate(d Diagnostic) StyleDiagnostic {
	return StyleDiagnostic{
		Position: s.position(d.Pos),
		Severity: d.Severity,
		Code:     d.Code,
		Message:  d.Message,
	}
}

func (s styleSource) position(pos lex.Pos) token.Position {
	if s.tree != nil {
		p := s.tree.PositionAt(pos)
		return token.Position{Filename: s.filename, Offset: int(pos), Line: p.Line, Column: p.Column}
	}
	before := s.text[:pos]
	line := strings.Count(before, "\n") + 1
	column := len(before) - strings.LastIndexByte(before, '\n')
	return token.Position{Filename: s.filename, Offset: int(pos), Line: line, Column: column}
}

func templateSource(t *template.Tree) styleSource {
	return styleSource{tree: t, filename: cmp.Or(t.Filepath, t.PositionAt(0).ParseName)}
}

// styleBlocks returns the <style> elements of trees, each once even when
// several trees hold it, and sheets.
func styleBlocks(trees []*template.Tree, sheets []Stylesheet) []styleBlock {
	var blocks []styleBlock
	seen := make(map[token.Position]bool)
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		source := templateSource(t)
		template.Walk(t, func(_ *template.Tree, n template.Node) {
			el, ok := n.(*template.HTMLElementNode)
			if !ok || !strings.EqualFold(el.TagName, "style") || len(el.Children) == 0 {
				return
			}
			css := templateCSS(el.Children)
			key := token.Position{Filename: source.filename, Offset: int(css.base)}
			if seen[key] {
				return
			}
			seen[key] = true
			blocks = append(blocks, styleBlock{source: source, css: css})
		})
	}
	for _, sheet := range sheets {
		blocks = append(blocks, styleBlock{
			source: styleSource{filename: sheet.Filename, text: sheet.Text},
			css:    cssText{text: sheet.Text},
		})
	}
	return blocks
}
