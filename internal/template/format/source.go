package format

import (
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// The helpers below return the source text of each token the printer
// writes, so it can copy a token as written instead of rebuilding it
// (spec §3 item 9): spacing, trim markers, backtick names, custom
// delimiters, tag-name case and void slashes all survive. Each is a
// slice of the node's String(), which is its source text; a node's
// positions index the same file text, so a slice between two of them
// is the source between them.

// sourceNode is a node whose String() is its source text, starting at
// Start().
type sourceNode interface {
	Start() lex.Pos
	String() string
}

// nodeSource returns the source between file positions from and to,
// both within n.
func nodeSource(n sourceNode, from, to lex.Pos) string {
	return n.String()[from-n.Start() : to-n.Start()]
}

// branchOpener returns b's opening action, from b's start to its body:
// {{if .A}}, {{- range $i, $v := .L -}}. For a branch chained onto an
// else (see BranchNode.ElseChain) the opener is the {{else if .B}} or
// {{else with .B}} action, which branchElse of the outer branch also
// returns.
func branchOpener(b *template.BranchNode) string {
	return nodeSource(b, b.Start(), b.List.Start())
}

// branchElse returns b's else action, from the end of its body to the
// start of what follows it: {{else}}, or for a chained else the whole
// {{else if .B}} / {{else with .B}} action up to the chained branch's
// body. It is empty when b has no else.
func branchElse(b *template.BranchNode) string {
	if b.ElseList == nil {
		return ""
	}
	if chain := b.ElseChain(); chain != nil {
		return nodeSource(b, b.List.End(), chain.List.Start())
	}
	return nodeSource(b, b.List.End(), b.ElseList.Start())
}

// branchEnd returns b's {{end}} action, from the end of its last list
// to b's end. It is empty when b's else is chained: the shared {{end}}
// belongs to the innermost branch of the chain.
func branchEnd(b *template.BranchNode) string {
	if b.ElseChain() != nil {
		return ""
	}
	last := b.List
	if b.ElseList != nil {
		last = b.ElseList
	}
	return nodeSource(b, last.End(), b.End())
}

// defineOpener returns the {{define "name"}} action, from d's start to
// its body.
func defineOpener(d *template.DefineNode) string {
	return nodeSource(d, d.Start(), d.Body.Start())
}

// defineEnd returns d's {{end}} action, from the end of its body to d's
// end.
func defineEnd(d *template.DefineNode) string {
	return nodeSource(d, d.Body.End(), d.End())
}

// blockOpener returns the {{block "name" pipeline}} action, from b's
// start to its body.
func blockOpener(b *template.BlockNode) string {
	return nodeSource(b, b.Start(), b.Body.Start())
}

// blockEnd returns b's {{end}} action, from the end of its body to b's
// end.
func blockEnd(b *template.BlockNode) string {
	return nodeSource(b, b.Body.End(), b.End())
}

// startTagClose returns the end of el's start tag, from the end of its
// last attribute (or of its tag name when it has none) through the
// '>': ">", " >", "/>", " />". It holds the whitespace before the '>'
// and a void element's slash.
func startTagClose(el *template.HTMLElementNode) string {
	from := el.AttrStart
	if len(el.Attr) > 0 {
		from = el.Attr[len(el.Attr)-1].End()
	}
	return nodeSource(el, from, el.AttrEnd)
}

// closingTag returns el's end tag as written, from its '<' to el's end:
// </div>, </DIV >. It is empty for a void element and for one with no
// end tag of its own.
func closingTag(el *template.HTMLElementNode) string {
	if el.Void || !el.Closed {
		return ""
	}
	return nodeSource(el, el.EndTagStart, el.End())
}

// actionSource returns the whole action n as written, delimiters and
// trim markers included. n is an action, a {{template}} call, a
// {{break}} or a {{continue}}, whose String() is its source text.
func actionSource(n template.Node) string {
	return n.String()
}
