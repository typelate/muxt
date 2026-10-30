package format

import (
	"go/token"
	"strings"

	"github.com/typelate/muxt/internal/template"
)

// DirectiveError is a muxt:fmt comment the formatter cannot apply. The
// file is left unchanged.
type DirectiveError struct {
	Position token.Position // Filename is Source's filename
	Message  string
}

func (e *DirectiveError) Error() string {
	return e.Position.String() + ": " + e.Message
}

const directivePrefix = "muxt:fmt"

// directiveVerbs are the muxt:fmt verbs. They are escape hatches, not
// configuration (spec amendment, gofmt model): there are no others.
const (
	verbOff      = "off"
	verbOn       = "on"
	verbPreserve = "preserve"
)

// region is source the printer writes as it is: the bytes from the end
// of the directive comment that opens it up to end. Items of the
// directive's list up to the one at index last, and the gaps inside the
// region, are not printed.
type region struct {
	end   int  // offset in the source of the end of the region
	last  int  // index in the list's items of the last item in the region
	toEnd bool // an off no on ends: the region runs to the end of its list
}

// directives are the verbatim regions of a file.
type directives struct {
	src                   string
	leftDelim, rightDelim string
	regions               map[*template.CommentNode]region
	// skipGaps are the gaps of a list that lie inside a region.
	skipGaps map[gapKey]bool
}

// scanDirectives finds the muxt:fmt comments in root (parsed from src),
// checks them, and returns the regions they open. It reports the first
// directive in the source that cannot be applied.
func scanDirectives(filename, src string, root *template.ListNode, leftDelim, rightDelim string) (*directives, error) {
	s := &directiveScanner{
		filename: filename,
		d: &directives{
			src: src, leftDelim: leftDelim, rightDelim: rightDelim,
			regions:  make(map[*template.CommentNode]region),
			skipGaps: make(map[gapKey]bool),
		},
	}
	if err := s.list(root, root.Nodes); err != nil {
		return nil, err
	}
	return s.d, nil
}

type directiveScanner struct {
	filename string
	d        *directives
}

// flatten returns nodes with document fragments replaced by their
// children, whitespace-only text included.
func flatten(nodes []template.Node) []template.Node {
	var flat []template.Node
	for _, n := range nodes {
		if frag, ok := n.(*template.HTMLDocumentFragmentNode); ok {
			flat = append(flat, flatten(frag.Children)...)
			continue
		}
		flat = append(flat, n)
	}
	return flat
}

// directiveOf reports the muxt:fmt comment text of n, trimmed: ok is
// false for any other node.
func directiveOf(n template.Node) (c *template.CommentNode, text string, ok bool) {
	c, ok = n.(*template.CommentNode)
	if !ok {
		return nil, "", false
	}
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(c.Text, "/*"), "*/"))
	return c, text, strings.HasPrefix(text, directivePrefix)
}

// verbOf returns the verb of a directive comment, or "" when its text is
// not exactly muxt:fmt and a verb.
func verbOf(text string) string {
	fields := strings.Fields(text)
	if len(fields) != 2 || fields[0] != directivePrefix {
		return ""
	}
	switch fields[1] {
	case verbOff, verbOn, verbPreserve:
		return fields[1]
	}
	return ""
}

func (s *directiveScanner) errorf(c *template.CommentNode, message string) error {
	return &DirectiveError{Position: s.position(s.d.commentStart(c)), Message: message}
}

// commentStart is the offset of the left delimiter of c.
func (d *directives) commentStart(c *template.CommentNode) int {
	at := int(c.Start()) - len(d.leftDelim)
	if c.TrimLeft {
		at -= len("- ")
	}
	return at
}

// commentEnd is the offset after the right delimiter of c.
func (d *directives) commentEnd(c *template.CommentNode) int {
	at := int(c.End()) + len(d.rightDelim)
	if c.TrimRight {
		at += len(" -")
	}
	return at
}

func (s *directiveScanner) position(offset int) token.Position {
	before := s.d.src[:max(0, min(offset, len(s.d.src)))]
	line := strings.Count(before, "\n") + 1
	column := len(before) - strings.LastIndex(before, "\n")
	return token.Position{Filename: s.filename, Offset: offset, Line: line, Column: column}
}

// spanEnd is the offset after the last byte of n's source, children of
// an element the source leaves unclosed included.
func (s *directiveScanner) spanEnd(n template.Node) int {
	switch n := n.(type) {
	case *template.CommentNode:
		return s.d.commentEnd(n)
	case *template.HTMLElementNode:
		if !n.Closed && !n.Void {
			if flat := flatten(n.Children); len(flat) > 0 {
				return max(int(n.End()), s.spanEnd(flat[len(flat)-1]))
			}
		}
	case *template.HTMLDocumentFragmentNode:
		if flat := flatten(n.Children); len(flat) > 0 {
			return s.spanEnd(flat[len(flat)-1])
		}
	}
	return int(n.End())
}

// list checks the directives in the child list key (its children are
// nodes) and the lists inside it.
func (s *directiveScanner) list(key template.Node, nodes []template.Node) error {
	flat := flatten(nodes)
	items, _ := splitChildren(nodes)
	itemIndex := make(map[template.Node]int, len(items))
	for i, item := range items {
		itemIndex[item] = i
	}
	// lastItem is the index of the last item at or before flat[at]:
	// whitespace-only text is no item.
	lastItem := func(at int) int {
		for ; at >= 0; at-- {
			if i, ok := itemIndex[flat[at]]; ok {
				return i
			}
		}
		return -1
	}
	var off *template.CommentNode // the open muxt:fmt off, or nil
	for at, n := range flat {
		if err := s.inside(n); err != nil {
			return err
		}
		c, text, ok := directiveOf(n)
		if !ok {
			continue
		}
		switch verbOf(text) {
		case verbOff:
			if off == nil {
				off = c
			}
		case verbOn:
			if off == nil {
				return s.errorf(c, "muxt:fmt on without muxt:fmt off before it in the same element")
			}
			s.d.regions[off] = region{end: s.d.commentStart(c), last: lastItem(at - 1)}
			s.skipRange(key, itemIndex[off]+1, itemIndex[c])
			off = nil
		case verbPreserve:
			if off != nil {
				continue // already verbatim
			}
			next := nextSibling(flat, at)
			if next < 0 {
				return s.errorf(c, "muxt:fmt preserve has no node after it")
			}
			k := itemIndex[flat[next]]
			s.d.regions[c] = region{end: s.spanEnd(flat[next]), last: k}
			s.skipRange(key, itemIndex[c]+1, k)
			if _, isText := flat[next].(*template.TextNode); isText {
				// The text's trailing run is the gap after it.
				s.d.skipGaps[gapKey{key, k + 1}] = true
			}
		default:
			return s.errorf(c, `unknown directive "`+text+`": want muxt:fmt off, muxt:fmt on, or muxt:fmt preserve`)
		}
	}
	if off != nil {
		s.d.regions[off] = region{end: s.spanEnd(flat[len(flat)-1]), last: lastItem(len(flat) - 1), toEnd: true}
		s.skipRange(key, itemIndex[off]+1, len(items))
	}
	return nil
}

// nextSibling returns the index in flat of the node after flat[at] that
// preserve applies to, skipping whitespace and comments, or -1.
func nextSibling(flat []template.Node, at int) int {
	for i := at + 1; i < len(flat); i++ {
		switch n := flat[i].(type) {
		case *template.CommentNode, *template.HTMLCommentNode:
			continue
		case *template.TextNode:
			if isRunOnly(n.Text) {
				continue
			}
		}
		return i
	}
	return -1
}

// inside checks the lists in n.
func (s *directiveScanner) inside(n template.Node) error {
	switch n := n.(type) {
	case *template.HTMLElementNode:
		return s.list(n, n.Children)
	case *template.IfNode:
		return s.branch(&n.BranchNode)
	case *template.WithNode:
		return s.branch(&n.BranchNode)
	case *template.RangeNode:
		return s.branch(&n.BranchNode)
	case *template.DefineNode:
		return s.body(n.Body)
	case *template.BlockNode:
		return s.body(n.Body)
	}
	return nil
}

func (s *directiveScanner) branch(b *template.BranchNode) error {
	if err := s.body(b.List); err != nil {
		return err
	}
	return s.body(b.ElseList)
}

func (s *directiveScanner) body(l *template.ListNode) error {
	if l == nil {
		return nil
	}
	return s.list(l, l.Nodes)
}

// skipRange marks gaps from through to of key as inside a region.
func (s *directiveScanner) skipRange(key template.Node, from, to int) {
	for i := from; i <= to; i++ {
		s.d.skipGaps[gapKey{key, i}] = true
	}
}

// startsWithWholeFile reports whether the first thing in the file is a muxt:fmt
// preserve, or a muxt:fmt off that nothing ends (spec §7.2).
func (d *directives) startsWithWholeFile(root *template.ListNode) bool {
	for _, n := range flatten(root.Nodes) {
		if text, ok := n.(*template.TextNode); ok && isRunOnly(text.Text) {
			continue
		}
		c, text, ok := directiveOf(n)
		if !ok {
			return false
		}
		return verbOf(text) == verbPreserve || d.regions[c].toEnd
	}
	return false
}
