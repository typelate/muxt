package analysis

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/lex"
)

// Comment diagnostic codes.
const (
	DiagCommentInRCDATA template.DiagnosticCode = "comment-in-rcdata"
	DiagEscapedMarkup   template.DiagnosticCode = "escaped-markup"
	DiagAbruptComment   template.DiagnosticCode = "abrupt-comment"
)

const (
	commentOpen  = "<!--"
	commentClose = "-->"
)

// IsDoctype reports whether n is a doctype, the only node the parser
// reads as a doctype that html/template writes unchanged. The parser
// reads a bogus comment or processing instruction (<!x>, <?x>) as a
// doctype node too; html/template writes its < as &lt;.
func IsDoctype(n *template.HTMLDoctypeNode) bool {
	return strings.HasPrefix(strings.ToUpper(n.Text), "<!DOCTYPE")
}

// CommentDelimitedAlike reports whether html/template ends comment n
// where the template parser does. html/template strips a comment from
// <!-- to the first --> after it, while the parser follows the HTML
// tokenizer, which also closes one abruptly (<!-->, <!--->).
func CommentDelimitedAlike(n *template.HTMLCommentNode) bool {
	src := n.String()
	return len(src) >= len("<!---->") && strings.HasSuffix(src, commentClose) &&
		strings.Index(src[len(commentOpen):], commentClose) == len(src)-len("<!---->")
}

// CheckComments reports, in the HTML text of t, comments and markup that
// html/template writes as text a browser shows, and comments html/template
// ends somewhere other than where a browser does:
//
//   - a <!-- in <title> or <textarea> content, which html/template escapes
//     there;
//   - a <! not starting a comment or doctype, or a <?, which html/template
//     escapes;
//   - a comment a browser ends abruptly (<!-->, <!--->), which
//     html/template strips through the next -->.
//
// Attribute values, <script> and <style> are not checked, and neither are
// the bodies of the {{define}} and {{block}} clauses in t: they are
// templates of their own.
//
// t must be parsed as HTML; a text/template set's templates are text.
func CheckComments(t *template.Tree) Diagnostics {
	if t == nil || t.Root == nil {
		return nil
	}
	c := commentChecker{tree: t}
	c.collectDefines()
	c.nodes(t.Root.Nodes)
	c.diags.SortByPos()
	return c.diags
}

type commentChecker struct {
	tree  *template.Tree
	diags Diagnostics
	// defines are the {{define}} and {{block}} clauses in the tree; what
	// they hold is not the tree's text.
	defines []template.Node
}

func (c *commentChecker) collectDefines() {
	template.Walk(c.tree, func(_ *template.Tree, n template.Node) {
		switch n.(type) {
		case *template.DefineNode, *template.BlockNode:
			c.defines = append(c.defines, n)
		}
	})
}

func (c *commentChecker) nodes(nodes []template.Node) {
	for _, n := range nodes {
		c.node(n)
	}
}

func (c *commentChecker) node(n template.Node) {
	switch n := n.(type) {
	case *template.ListNode:
		if n != nil {
			c.nodes(n.Nodes)
		}
	case *template.HTMLDocumentFragmentNode:
		c.nodes(n.Children)
	case *template.IfNode:
		c.node(n.List)
		c.node(n.ElseList)
	case *template.RangeNode:
		c.node(n.List)
		c.node(n.ElseList)
	case *template.WithNode:
		c.node(n.List)
		c.node(n.ElseList)
	case *template.HTMLElementNode:
		// Attributes are not walked: html/template keeps a comment in an
		// attribute value.
		tag := strings.ToLower(n.TagName)
		switch {
		case tag == "title" || tag == "textarea":
			c.rcdata(tag, n.Children)
		case rawTextElements[tag]:
		default:
			c.nodes(n.Children)
		}
	case *template.HTMLDoctypeNode:
		if !IsDoctype(n) && !strings.HasPrefix(n.Text, commentOpen) {
			c.report(n.Start(), DiagEscapedMarkup, msgEscapedMarkup, n.Text[:len("<!")])
		}
	case *template.HTMLCommentNode:
		if !CommentDelimitedAlike(n) {
			c.abruptComment(n)
		}
	case *template.TextNode:
		c.text(n)
	}
}

// rcdata reports each <!-- in the text of a title or textarea element,
// including text inside actions' branches.
func (c *commentChecker) rcdata(tag string, nodes []template.Node) {
	for _, n := range nodes {
		switch n := n.(type) {
		case *template.TextNode:
			for i := range indexes(n.Text, commentOpen) {
				c.report(n.Start()+lex.Pos(i), DiagCommentInRCDATA, msgCommentInRCDATA, tag)
			}
		case *template.ListNode:
			if n != nil {
				c.rcdata(tag, n.Nodes)
			}
		case *template.IfNode:
			c.rcdata(tag, branchNodes(n.List, n.ElseList))
		case *template.RangeNode:
			c.rcdata(tag, branchNodes(n.List, n.ElseList))
		case *template.WithNode:
			c.rcdata(tag, branchNodes(n.List, n.ElseList))
		}
	}
}

func branchNodes(lists ...*template.ListNode) []template.Node {
	var nodes []template.Node
	for _, l := range lists {
		if l != nil {
			nodes = append(nodes, l)
		}
	}
	return nodes
}

// text reports each <! not starting a comment or doctype, and each <?, in
// text the parser did not read as markup, such as a <! an action follows.
// One inside a quoted attribute value of a tag the parser read as text
// (<li{{if .X}} title="<!x>"{{end}}>) is skipped: html/template keeps it.
func (c *commentChecker) text(n *template.TextNode) {
	text := n.Text
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '<' {
			continue
		}
		rest := text[i+1:]
		switch {
		case rest[0] == '?':
		case rest[0] != '!':
			continue
		case strings.HasPrefix(string(rest[1:]), "--"),
			len(rest) >= len("!DOCTYPE") && strings.EqualFold(string(rest[1:len("!DOCTYPE")]), "DOCTYPE"):
			continue
		}
		if inQuotedValue(text[:i]) {
			continue
		}
		c.report(n.Start()+lex.Pos(i), DiagEscapedMarkup, msgEscapedMarkup, text[i:i+2])
	}
}

// inQuotedValue reports whether text, after its last >, leaves an
// attribute value open: an = then a quote the text does not close.
func inQuotedValue(text []byte) bool {
	text = text[bytes.LastIndexByte(text, '>')+1:]
	var quote byte
	for i := 0; i < len(text); i++ {
		switch c := text[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '=':
			j := i + 1
			for j < len(text) && isHTMLSpace(text[j]) {
				j++
			}
			if j < len(text) && (text[j] == '"' || text[j] == '\'') {
				quote = text[j]
				i = j
			}
		}
	}
	return quote != 0
}

func isHTMLSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}

// abruptComment reports a comment a browser ends at its <!--> or <!--->,
// with where html/template ends it: at the first --> after the <!--,
// passing over define bodies, which are not the tree's text.
func (c *commentChecker) abruptComment(n *template.HTMLCommentNode) {
	src := n.String()
	from := n.Start() + lex.Pos(len(commentOpen))
	rest := c.tree.Root.String()
	rootStart := c.tree.Root.Start()
	for {
		if from < rootStart || int(from-rootStart) > len(rest) {
			break
		}
		i := strings.Index(rest[from-rootStart:], commentClose)
		if i < 0 {
			break
		}
		at := from + lex.Pos(i)
		if d := c.defineAt(at); d != nil {
			from = d.End()
			continue
		}
		p := c.tree.PositionAt(at)
		c.report(n.Start(), DiagAbruptComment, msgAbruptComment, src, p.Line, p.Column)
		return
	}
	c.report(n.Start(), DiagAbruptComment, msgAbruptCommentUnclosed, src)
}

func (c *commentChecker) defineAt(pos lex.Pos) template.Node {
	for _, d := range c.defines {
		if d.Start() <= pos && pos < d.End() {
			return d
		}
	}
	return nil
}

func (c *commentChecker) report(pos lex.Pos, code template.DiagnosticCode, format string, args ...any) {
	c.diags = append(c.diags, Diagnostic{
		Pos:      pos,
		Severity: SeverityError,
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
	})
}

// indexes yields the offset of each occurrence of sep in text.
func indexes(text []byte, sep string) func(func(int) bool) {
	return func(yield func(int) bool) {
		for off := 0; ; {
			i := strings.Index(string(text[off:]), sep)
			if i < 0 || !yield(off+i) {
				return
			}
			off += i + len(sep)
		}
	}
}
