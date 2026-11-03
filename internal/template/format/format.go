// Package format reindents Go HTML template source for muxt fmt. It
// changes only the leading whitespace of lines that already exist,
// using the syntax tree of muxt's own template parser for depth and the
// whitespace classifier for which runs the browser lets it change.
package format

import (
	"strings"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// Option configures the formatter.
type Option func(*formatter)

// Delimiters sets the left and right template delimiters.
// Defaults to "{{" and "}}".
func Delimiters(left, right string) Option {
	return func(f *formatter) {
		f.leftDelim = left
		f.rightDelim = right
	}
}

// Styles sets the author CSS the formatter classifies whitespace with:
// stylesheet files the templates use, templates whose <style> elements
// apply, and whether that is all the author CSS (see
// analysis.WhitespaceOptions). An element whose white-space that CSS
// makes preserve, or leaves unknown, is written as it is in the source.
// Without Styles, or without AuthorCSSKnown, an element with a class or
// id is unknown and is written as it is.
func Styles(options analysis.WhitespaceOptions) Option {
	return func(f *formatter) {
		f.whitespace = options
	}
}

// Node formats the given AST node, which must come from a parsed tree,
// as Source formats a file: it reindents the lines the source already
// has, one tab per level, and copies every other byte from the node's
// source. It sees only the author CSS that Styles gives and the tree's
// own <style> elements. A tree holding markup the template parser read
// as text but a browser reads as a tag, or a self-closed HTML element
// that is not void, is written as it is, or only the define body
// holding the element when no {{template}} in the tree calls it (see
// hasUnparsedMarkup and ignoredSelfCloses).
func Node(root template.Node, opts ...Option) string {
	list, ok := root.(*template.ListNode)
	if !ok {
		list = &template.ListNode{Nodes: []template.Node{root}}
	}
	return newFormatter(opts).format(root.String(), root.Start(), list, nil)
}

func newFormatter(opts []Option) *formatter {
	f := &formatter{leftDelim: "{{", rightDelim: "}}"}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

type formatter struct {
	leftDelim  string
	rightDelim string
	whitespace analysis.WhitespaceOptions
}

// format reindents src, the source of list starting at position base.
// dir holds the file's muxt:fmt regions; it is nil for Node.
func (f *formatter) format(src string, base lex.Pos, list *template.ListNode, dir *directives) string {
	whole, bodies := ignoredSelfCloses(list)
	if whole || hasUnparsedMarkup(list.Nodes) {
		return src
	}
	styles := analysis.ComputeWhitespace([]*template.Tree{{Root: list}}, f.whitespace)
	if unsureMisnested(list, styles) {
		return src
	}
	unsureReopened(list, styles)
	unsureParsedElsewhere(list, styles, f.whitespace.Context)
	return newEmitter(src, base, classify(list, styles), bodies, dir).emit(list)
}

// Source formats a Go HTML template file as Node does, with the same
// options: it reads the author CSS Styles gives and the file's <style>
// elements. Only the whitespace after the last line break of a run the
// browser collapses (or a trim marker removes) changes; every other
// byte, line breaks included (CR LF stays CR LF), is the source's, so
// the file's final whitespace and newline are as written. A file with
// no HTML element is not a page (a plain-text email, a text-only
// partial), so its whitespace is all preserved and Source returns it
// unchanged.
func Source(filename, src string, opts ...Option) (string, error) {
	f := newFormatter(opts)
	tree, err := template.ParseTree(filename, filename, src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
		LeftDelim:   f.leftDelim,
		RightDelim:  f.rightDelim,
	})
	if err != nil {
		return "", err
	}
	dir, err := scanDirectives(filename, src, tree.Root, f.leftDelim, f.rightDelim)
	if err != nil {
		return "", err
	}
	if !hasElement(tree) || dir.startsWithWholeFile(tree.Root) {
		return src, nil
	}
	return f.format(src, 0, tree.Root, dir), nil
}

// ignoredSelfCloses finds the HTML elements that are not void written
// with a self-closing slash (<pre/>, <div/>). The browser ignores the
// slash outside SVG and MathML content, so the content after the tag is
// the element's in the browser's tree and its sibling in the template
// tree: the two disagree from there on, as with hasUnparsedMarkup. One
// inside a define body leaves that body as written (bodies); its other
// defines are independent templates. One outside any define, or in a
// define or block the file calls with {{template}} or that is a block
// (rendered in place), leaves the whole file as written (whole).
func ignoredSelfCloses(root *template.ListNode) (whole bool, bodies map[*template.DefineNode]bool) {
	s := selfCloseScan{bodies: make(map[*template.DefineNode]bool), names: make(map[string]bool)}
	s.scan(root.Nodes, false, new(bool), nil)
	if s.whole {
		return true, nil
	}
	template.Walk(&template.Tree{Root: root}, func(_ *template.Tree, n template.Node) {
		if call, ok := n.(*template.TemplateNode); ok && s.names[call.Name] {
			s.whole = true
		}
	})
	if s.whole {
		return true, nil
	}
	return false, s.bodies
}

type selfCloseScan struct {
	whole  bool
	bodies map[*template.DefineNode]bool
	names  map[string]bool // defines and blocks holding one
}

// scan walks nodes, which are SVG or MathML content when foreign. broke
// is shared by the content of one foreign root: once an HTML breakout
// start tag (HTML §13.2.6.5) pops the browser out of foreign content,
// what follows is HTML too. enclosing are the defines and blocks nodes
// are in, outermost first.
func (s *selfCloseScan) scan(nodes []template.Node, foreign bool, broke *bool, enclosing []template.Node) {
	for _, n := range nodes {
		switch n := n.(type) {
		case *template.HTMLDocumentFragmentNode:
			s.scan(n.Children, foreign, broke, enclosing)
		case *template.HTMLElementNode:
			tag := strings.ToLower(n.TagName)
			if foreign && breakoutTags[tag] {
				*broke = true
			}
			inForeign := foreign && !*broke
			childBroke := broke
			if !inForeign && (tag == "svg" || tag == "math") {
				inForeign = true
				childBroke = new(bool)
			}
			if n.Void && !voidElements[tag] && !inForeign {
				s.found(enclosing)
			}
			if inForeign && integrationPoints[tag] {
				s.scan(n.Children, false, new(bool), enclosing)
			} else {
				s.scan(n.Children, inForeign, childBroke, enclosing)
			}
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			b := branchOf(n)
			if b.List != nil {
				s.scan(b.List.Nodes, foreign, broke, enclosing)
			}
			if b.ElseList != nil {
				s.scan(b.ElseList.Nodes, foreign, broke, enclosing)
			}
		case *template.DefineNode:
			// A body is HTML content: its callers are unknown.
			if n.Body != nil {
				s.scan(n.Body.Nodes, false, new(bool), append(enclosing, n))
			}
		case *template.BlockNode:
			if n.Body != nil {
				s.scan(n.Body.Nodes, false, new(bool), append(enclosing, n))
			}
		}
	}
}

// found records an ignored self-close inside enclosing.
func (s *selfCloseScan) found(enclosing []template.Node) {
	var define *template.DefineNode
	for _, e := range enclosing {
		switch e := e.(type) {
		case *template.DefineNode:
			s.names[e.Name] = true
			define = e
		case *template.BlockNode:
			s.names[e.Name] = true
			s.whole = true // a block renders in place
		}
	}
	if define == nil {
		s.whole = true
		return
	}
	s.bodies[define] = true
}

// breakoutTags are the start tags that end SVG and MathML content (HTML
// §13.2.6.5, "any other start tag" in foreign content). font breaks out
// only with a color, face or size attribute; it is listed whatever its
// attributes, which errs toward HTML, the cautious side.
var breakoutTags = map[string]bool{
	"b": true, "big": true, "blockquote": true, "body": true, "br": true,
	"center": true, "code": true, "dd": true, "div": true, "dl": true,
	"dt": true, "em": true, "embed": true, "font": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"head": true, "hr": true, "i": true, "img": true, "li": true,
	"listing": true, "menu": true, "meta": true, "nobr": true, "ol": true,
	"p": true, "pre": true, "ruby": true, "s": true, "small": true,
	"span": true, "strong": true, "strike": true, "sub": true, "sup": true,
	"table": true, "tt": true, "u": true, "ul": true, "var": true,
}

// integrationPoints are the SVG and MathML elements whose content is
// HTML again (HTML §13.2.6.5): SVG foreignObject, desc and title, and
// MathML annotation-xml and text integration points. Matching them by
// local name only errs toward HTML, which is the cautious side.
var integrationPoints = map[string]bool{
	"foreignobject": true, "desc": true, "title": true,
	"mi": true, "mo": true, "mn": true, "ms": true, "mtext": true, "annotation-xml": true,
}

// hasUnparsedMarkup reports whether text in nodes, outside raw text and
// RCDATA elements and comments, holds markup the template parser read as
// text that changes how a browser tokenizes the rest of the file (see
// unreadMarkupIn). The two trees then disagree from there on, and the
// whitespace after it cannot be classified. Other unread markup is
// handled where it stands (unsureMisnested).
// A bogus comment or processing instruction the parser read as one node
// (<!x>, <?x>) is unparsed markup too: html/template writes its < as
// &lt;, so the browser reads what follows as text and markup, a raw text
// start tag (<!<style>) included. So is a comment that html/template
// delimits differently from the parser (delimitedAlike).
func hasUnparsedMarkup(nodes []template.Node) bool {
	for _, n := range nodes {
		switch n := n.(type) {
		case *template.HTMLDoctypeNode:
			if !isDoctype(n) {
				return true
			}
		case *template.HTMLCommentNode:
			if !delimitedAlike(n) {
				return true
			}
		case *template.TextNode:
			if unreadMarkupIn(n.Text) == unreadRetokenizes {
				return true
			}
		case *template.HTMLDocumentFragmentNode:
			if hasUnparsedMarkup(n.Children) {
				return true
			}
		case *template.HTMLElementNode:
			if !rawTextTags[strings.ToLower(n.TagName)] && hasUnparsedMarkup(n.Children) {
				return true
			}
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			b := branchOf(n)
			if b.List != nil && hasUnparsedMarkup(b.List.Nodes) || b.ElseList != nil && hasUnparsedMarkup(b.ElseList.Nodes) {
				return true
			}
		case *template.DefineNode:
			if n.Body != nil && hasUnparsedMarkup(n.Body.Nodes) {
				return true
			}
		case *template.BlockNode:
			if n.Body != nil && hasUnparsedMarkup(n.Body.Nodes) {
				return true
			}
		}
	}
	return false
}

// delimitedAlike reports whether html/template ends comment n where the
// template parser does. html/template strips a comment from <!-- to the
// first --> after it, while the parser follows the HTML tokenizer, which
// also closes one abruptly (<!-->, <!--->). Where they differ the parser
// splits text from the comment that html/template does not, and the
// formatter, which writes a comment with a --> close, would change what
// html/template strips, so the file is written as it is.
func delimitedAlike(n *template.HTMLCommentNode) bool {
	src := n.String()
	return len(src) >= len("<!---->") && strings.HasSuffix(src, "-->") &&
		strings.Index(src[len("<!--"):], "-->") == len(src)-len("<!---->")
}

// isDoctype reports whether n is a doctype, the only node the parser
// reads as a doctype that html/template writes unchanged.
func isDoctype(n *template.HTMLDoctypeNode) bool {
	return strings.HasPrefix(strings.ToUpper(n.Text), "<!DOCTYPE")
}

// unreadMarkup is what a < in text the template parser did not read as
// markup means to a browser.
type unreadMarkup uint8

const (
	unreadNone unreadMarkup = iota
	// unreadEndTag is an end tag the parser could not match (</b>).
	unreadEndTag
	// unreadTag is a start tag the parser could not read, such as
	// <li{{if .Done}} class="done"{{end}}>: an element the template tree
	// does not have.
	unreadTag
	// unreadRetokenizes changes how the rest of the file tokenizes: a
	// raw text or RCDATA start tag (<textarea </textarea>), a <! or <?
	// (bogus comment, CDATA, doctype), or </ then a non-letter.
	unreadRetokenizes
)

// unreadMarkupIn reports the strongest unread markup in text.
func unreadMarkupIn(text []byte) unreadMarkup {
	found := unreadNone
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '<' {
			continue
		}
		switch c := text[i+1]; {
		case c == '!', c == '?':
			return unreadRetokenizes
		case c == '/' && (i+2 >= len(text) || !isASCIILetter(text[i+2])):
			return unreadRetokenizes
		case c == '/':
			found = max(found, unreadEndTag)
		case isASCIILetter(c):
			end := i + 1
			for end < len(text) && !isTagNameEnd(text[end]) {
				end++
			}
			if rawTextTags[strings.ToLower(string(text[i+1:end]))] {
				return unreadRetokenizes
			}
			found = unreadTag
		}
	}
	return found
}

// isTagNameEnd reports whether c ends a tag name (HTML §13.2.5.8), or
// starts a template action that the parser split the tag at.
func isTagNameEnd(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '/', '>', '{':
		return true
	}
	return false
}

func isASCIILetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

// rawTextTags name the elements whose content the HTML tokenizer reads
// as text up to their end tag (raw text and RCDATA, HTML §13.1.2), with
// scripting enabled.
var rawTextTags = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true, "xmp": true,
	"iframe": true, "noembed": true, "noframes": true, "noscript": true, "plaintext": true,
}

func hasElement(tree *template.Tree) bool {
	found := false
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		if _, ok := n.(*template.HTMLElementNode); ok {
			found = true
		}
	})
	return found
}

// voidElements are the HTML elements with no content, whose start tag
// the formatter writes without a slash (as the template parser's list).
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true, "param": true,
	"source": true, "track": true, "wbr": true,
}
