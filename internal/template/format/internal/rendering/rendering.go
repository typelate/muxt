// Package rendering turns rendered HTML into a whitespace-normalized DOM
// token stream (spec 2026-10-29-fmt-whitespace-design §8.3), so tests can
// check that formatting a template does not change what a browser draws.
//
// The oracle is sound but not complete: it may call two outputs different
// when a browser draws them the same, but it never calls them equivalent
// when CSS Text 3 says they render differently.
package rendering

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// Context is the element a rendered fragment is parsed in.
type Context uint8

const (
	Body      Context = iota // html.ParseFragment with a <body> context (html.Parse when the output starts with a doctype or <html>)
	TableBody                // <tbody> context
)

// Kind is the kind of a Token.
type Kind uint8

const (
	Open    Kind = iota // start of an element
	Close               // end of an element (void elements too)
	Comment             // an HTML comment, exactly
	Text                // literal text: content characters, or raw text and preserved text exactly
	SP                  // a collapsible run of spaces and tabs
	SB                  // a collapsible run containing a segment break
	LF                  // a preserved line feed (white-space: pre-line)
	Doctype             // a doctype; Data is its name
)

func (k Kind) String() string {
	switch k {
	case Open:
		return "open"
	case Close:
		return "close"
	case Comment:
		return "comment"
	case Text:
		return "text"
	case SP:
		return "SP"
	case SB:
		return "SB"
	case LF:
		return "LF"
	case Doctype:
		return "doctype"
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}

// Token is one item of the normalized stream.
type Token struct {
	Kind Kind
	Data string           // tag name, comment text, or literal text
	Attr []html.Attribute // Open only, in source order
}

func (t Token) String() string {
	switch t.Kind {
	case SP, SB, LF:
		return t.Kind.String()
	case Open:
		var sb strings.Builder
		sb.WriteString("<" + t.Data)
		for _, a := range t.Attr {
			sb.WriteByte(' ')
			if a.Namespace != "" {
				sb.WriteString(a.Namespace + ":")
			}
			sb.WriteString(a.Key + "=" + strconv.Quote(a.Val))
		}
		sb.WriteByte('>')
		return sb.String()
	case Close:
		return "</" + t.Data + ">"
	}
	return t.Kind.String() + " " + strconv.Quote(t.Data)
}

func (t Token) equal(o Token) bool {
	if t.Kind != o.Kind || t.Data != o.Data || len(t.Attr) != len(o.Attr) {
		return false
	}
	for i := range t.Attr {
		if t.Attr[i] != o.Attr[i] {
			return false
		}
	}
	return true
}

// Tokens parses rendered as a browser would in ctx and returns its
// whitespace-normalized token stream. sheets are the stylesheet files the
// page uses besides its <style> elements and style attributes.
func Tokens(rendered string, ctx Context, sheets []analysis.Stylesheet) ([]Token, error) {
	nodes, err := parse(rendered, ctx)
	if err != nil {
		return nil, err
	}
	styles, err := classify(nodes, sheets)
	if err != nil {
		return nil, err
	}
	b := builder{styles: styles}
	return b.container(nodes, analysis.WhiteSpaceCollapse), nil
}

// Diff returns "" when a and b are equivalent, else a description of the
// first differing token with its index and a few tokens of context.
func Diff(a, b string, ctx Context, sheets []analysis.Stylesheet) (string, error) {
	ta, err := Tokens(a, ctx, sheets)
	if err != nil {
		return "", fmt.Errorf("tokens of a: %w", err)
	}
	tb, err := Tokens(b, ctx, sheets)
	if err != nil {
		return "", fmt.Errorf("tokens of b: %w", err)
	}
	i := 0
	for i < len(ta) && i < len(tb) && ta[i].equal(tb[i]) {
		i++
	}
	if i == len(ta) && i == len(tb) {
		return "", nil
	}
	return fmt.Sprintf("token %d differs:\n\ta: %s\n\tb: %s", i, window(ta, i), window(tb, i)), nil
}

// window describes tokens around i, marking token i.
func window(tokens []Token, i int) string {
	const context = 3
	var parts []string
	for j := max(0, i-context); j < min(len(tokens), i+context+1); j++ {
		s := tokens[j].String()
		if j == i {
			s = "[[" + s + "]]"
		}
		parts = append(parts, s)
	}
	if i >= len(tokens) {
		parts = append(parts, "[[end]]")
	}
	return strings.Join(parts, " ")
}

func parse(rendered string, ctx Context) ([]*html.Node, error) {
	if ctx == Body && startsDocument(rendered) {
		doc, err := html.Parse(strings.NewReader(rendered))
		if err != nil {
			return nil, fmt.Errorf("parsing document: %w", err)
		}
		var nodes []*html.Node
		for c := doc.FirstChild; c != nil; c = c.NextSibling {
			nodes = append(nodes, c)
		}
		return nodes, nil
	}
	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	if ctx == TableBody {
		context = &html.Node{Type: html.ElementNode, Data: "tbody", DataAtom: atom.Tbody}
	}
	nodes, err := html.ParseFragment(strings.NewReader(rendered), context)
	if err != nil {
		return nil, fmt.Errorf("parsing fragment: %w", err)
	}
	return nodes, nil
}

func startsDocument(s string) bool {
	s = strings.ToLower(strings.TrimLeft(s, " \t\n\f\r"))
	if strings.HasPrefix(s, "<!doctype") {
		return true
	}
	rest, ok := strings.CutPrefix(s, "<html")
	return ok && (rest == "" || strings.ContainsRune(" \t\n\f\r/>", rune(rest[0])))
}

// rawText are the raw text and RCDATA elements: their content is text
// whatever the CSS says.
var rawText = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true,
	"xmp": true, "iframe": true, "noembed": true, "noframes": true,
	"noscript": true, "plaintext": true,
}

// unparsedRawText are raw text elements muxt's template parser reads as
// markup. classify renders them empty so the two trees stay in step.
var unparsedRawText = map[string]bool{
	"xmp": true, "iframe": true, "noembed": true, "noframes": true,
	"noscript": true, "plaintext": true,
}

// classify computes each element's display and white-space with the same
// cascade the formatter uses: it renders nodes, parses the result with muxt's
// template parser (with delimiters that cannot occur in rendered HTML), and
// pairs the two trees' elements in document order.
func classify(nodes []*html.Node, sheets []analysis.Stylesheet) (map[*html.Node]analysis.Whitespace, error) {
	var sb strings.Builder
	for _, n := range nodes {
		if err := html.Render(&sb, skeleton(n)); err != nil {
			return nil, fmt.Errorf("rendering parsed HTML: %w", err)
		}
	}
	options := lex.NewOptions()
	options.LeftDelim, options.RightDelim = "\x00{", "}\x00"
	tree, err := template.ParseTree("rendered.html", "rendered", sb.String(), nil, options)
	if err != nil {
		return nil, fmt.Errorf("parsing rendered HTML as a template: %w", err)
	}
	computed := analysis.ComputeWhitespace([]*template.Tree{tree}, analysis.WhitespaceOptions{Sheets: sheets, AuthorCSSKnown: true})

	var elements []*template.HTMLElementNode
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		if el, ok := n.(*template.HTMLElementNode); ok {
			elements = append(elements, el)
		}
	})
	var parsed []*html.Node
	for _, n := range nodes {
		collectElements(&parsed, n)
	}
	if len(parsed) != len(elements) {
		return nil, fmt.Errorf("the template parser found %d elements, x/net/html %d", len(elements), len(parsed))
	}
	styles := make(map[*html.Node]analysis.Whitespace, len(parsed))
	for i, n := range parsed {
		if !strings.EqualFold(elements[i].TagName, n.Data) {
			return nil, fmt.Errorf("element %d is <%s> to the template parser, <%s> to x/net/html", i, elements[i].TagName, n.Data)
		}
		ws, ok := computed[elements[i]]
		if !ok {
			ws = analysis.Whitespace{Display: analysis.DisplayUnknown, WhiteSpace: analysis.WhiteSpaceUnknown}
		}
		styles[n] = ws
	}
	return styles, nil
}

func collectElements(out *[]*html.Node, n *html.Node) {
	if n.Type == html.ElementNode {
		*out = append(*out, n)
		if unparsedRawText[n.Data] {
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectElements(out, c)
	}
}

// skeleton copies n, leaving out the content of unparsedRawText elements.
func skeleton(n *html.Node) *html.Node {
	c := &html.Node{Type: n.Type, DataAtom: n.DataAtom, Data: n.Data, Namespace: n.Namespace, Attr: n.Attr}
	if n.Type == html.ElementNode && unparsedRawText[n.Data] {
		return c
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.AppendChild(skeleton(ch))
	}
	return c
}

// class is how an item takes part in its inline formatting context.
type class uint8

const (
	content     class = iota // text, atomic inlines: ends a collapsible sequence
	collapsible              // SP or SB
	boundary                 // an inline box's open or close: collapsing passes through
	transparent              // comments, display:none and doctypes: neighbors look through
	edge                     // a block-level or table-internal box, br, or a preserved LF: a line edge
)

type item struct {
	class  class
	tokens []Token
}

type builder struct {
	styles map[*html.Node]analysis.Whitespace
}

// container returns the tokens of the content of a block container: the
// children form one inline formatting context, with nested block boxes as
// line edges.
func (b *builder) container(children []*html.Node, mode analysis.WhiteSpace) []Token {
	var items []item
	for _, c := range children {
		items = b.flow(items, c, mode)
	}
	return resolve(items)
}

func (b *builder) childContainer(n *html.Node, mode analysis.WhiteSpace) []Token {
	var children []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		children = append(children, c)
	}
	return b.container(children, mode)
}

// flow appends the items of n, whose parent has white-space mode.
func (b *builder) flow(items []item, n *html.Node, mode analysis.WhiteSpace) []item {
	switch n.Type {
	case html.TextNode:
		return appendText(items, n.Data, mode)
	case html.CommentNode:
		return append(items, item{class: transparent, tokens: []Token{{Kind: Comment, Data: n.Data}}})
	case html.DoctypeNode:
		return append(items, item{class: transparent, tokens: []Token{{Kind: Doctype, Data: n.Data}}})
	case html.ElementNode:
	default:
		return items
	}

	ws := b.styles[n]
	open := Token{Kind: Open, Data: n.Data, Attr: append([]html.Attribute(nil), n.Attr...)}
	closing := Token{Kind: Close, Data: n.Data}

	var outer class
	switch ws.Display {
	case analysis.DisplayBlock, analysis.DisplayTableInternal, analysis.DisplayNewline:
		outer = edge
	case analysis.DisplayNone:
		outer = transparent
	case analysis.DisplayAtomic:
		outer = content
	default: // inline, contents, and unknown (treated as inline)
		outer = boundary
	}

	if rawText[n.Data] {
		tokens := []Token{open}
		var text strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			text.WriteString(c.Data)
		}
		if text.Len() > 0 {
			tokens = append(tokens, Token{Kind: Text, Data: text.String()})
		}
		tokens = append(tokens, closing)
		if outer == boundary {
			outer = content // its text does not collapse with its neighbors
		}
		return append(items, item{class: outer, tokens: tokens})
	}

	if outer == boundary {
		items = append(items, item{class: boundary, tokens: []Token{open}})
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			items = b.flow(items, c, ws.WhiteSpace)
		}
		return append(items, item{class: boundary, tokens: []Token{closing}})
	}
	tokens := append([]Token{open}, b.childContainer(n, ws.WhiteSpace)...)
	tokens = append(tokens, closing)
	return append(items, item{class: outer, tokens: tokens})
}

// appendText splits text into items by the white-space mode of its parent.
func appendText(items []item, text string, mode analysis.WhiteSpace) []item {
	if text == "" {
		return items
	}
	if mode != analysis.WhiteSpaceCollapse && mode != analysis.WhiteSpacePreserveBreaks {
		// preserve, and unknown (treated as preserve, the strictest)
		return append(items, item{class: content, tokens: []Token{{Kind: Text, Data: text}}})
	}
	for text != "" {
		n := strings.IndexFunc(text, isDocumentWhiteSpace)
		if n < 0 {
			n = len(text)
		}
		if n > 0 {
			items = append(items, item{class: content, tokens: []Token{{Kind: Text, Data: text[:n]}}})
			text = text[n:]
			continue
		}
		n = strings.IndexFunc(text, func(r rune) bool { return !isDocumentWhiteSpace(r) })
		if n < 0 {
			n = len(text)
		}
		run := text[:n]
		text = text[n:]
		breaks := strings.Count(run, "\n")
		switch {
		case breaks == 0:
			items = append(items, item{class: collapsible, tokens: []Token{{Kind: SP}}})
		case mode == analysis.WhiteSpacePreserveBreaks:
			for range breaks {
				items = append(items, item{class: edge, tokens: []Token{{Kind: LF}}})
			}
		default:
			items = append(items, item{class: collapsible, tokens: []Token{{Kind: SB}}})
		}
	}
	return items
}

// isDocumentWhiteSpace reports CSS Text 3 document white space. The parser
// has already turned CR into LF; FF is content.
func isDocumentWhiteSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// resolve applies the collapsing rules of §8.3 rule 5 to one inline
// formatting context and flattens it.
func resolve(items []item) []Token {
	// a, b: a collapsible token after another of the same kind, with only
	// inline box boundaries and transparent items between, is deleted (the
	// first keeps its box). An SP next to an SB is kept: whether Phase I
	// step 1 removes it across an inline boundary, and what the segment
	// break becomes (§4.1.3), is UA-defined, so keeping both is the
	// conservative choice.
	kept := items[:0:0]
	var last Kind
	pending := false
	for _, it := range items {
		switch it.class {
		case collapsible:
			k := it.tokens[0].Kind
			if pending && last == k {
				continue
			}
			last, pending = k, true
		case boundary, transparent:
		default:
			pending = false
		}
		kept = append(kept, it)
	}

	// c, d: collapsible tokens at a line edge are deleted, looking through
	// inline box boundaries and transparent items. This also drops
	// whitespace between table-internal boxes.
	drop := make([]bool, len(kept))
	for i := 0; i < len(kept); {
		if !passable(kept[i].class) {
			i++
			continue
		}
		j := i
		for j < len(kept) && passable(kept[j].class) {
			j++
		}
		if i == 0 || kept[i-1].class == edge || j == len(kept) || kept[j].class == edge {
			for k := i; k < j; k++ {
				drop[k] = kept[k].class == collapsible
			}
		}
		i = j
	}

	var tokens []Token
	for i, it := range kept {
		if !drop[i] {
			tokens = append(tokens, it.tokens...)
		}
	}
	return tokens
}

func passable(c class) bool {
	return c == collapsible || c == boundary || c == transparent
}
