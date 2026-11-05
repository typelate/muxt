package analysis

import (
	"cmp"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/lex"
)

// Comment diagnostic codes.
const (
	DiagCommentInRCDATA  template.DiagnosticCode = "comment-in-rcdata"
	DiagEscapedMarkup    template.DiagnosticCode = "escaped-markup"
	DiagCommentDelimiter template.DiagnosticCode = "comment-delimiter"
	DiagCommentInRawText template.DiagnosticCode = "comment-in-raw-text"
	DiagEscapedInRawText template.DiagnosticCode = "escaped-markup-in-raw-text"
	DiagEscaperRefuses   template.DiagnosticCode = "escaper-refuses"
)

const (
	commentOpen  = "<!--"
	commentClose = "-->"

	// derivedTemplateSep starts the suffix html/template appends to the
	// name of a template it escapes for a context other than text.
	derivedTemplateSep = "$htmltemplate_"
	// derivedForComment is the suffix of one called in an HTML comment.
	derivedForComment = derivedTemplateSep + "stateHTMLCmt"
)

// html/template special-cases only script and style among the elements
// whose content a browser reads as raw text. It reads the content of the
// others as HTML text: it strips the comments in it and escapes its <!.
var (
	// rawTextShown are those whose content browsers show, comments and
	// "&lt;" included.
	rawTextShown = map[string]bool{"xmp": true, "plaintext": true}
	// rawTextHidden are those whose content browsers do not render.
	// noscript is not one: with scripting off its content is HTML.
	rawTextHidden = map[string]bool{"iframe": true, "noembed": true, "noframes": true}
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

// TreeDiagnostic is a diagnostic at a position in the text of Tree.
type TreeDiagnostic struct {
	Tree *template.Tree
	Diagnostic
}

// CheckComments reports where html/template writes the HTML templates of
// a set otherwise than a browser reads their source. It runs the standard
// library's html/template escaper over the same sources and compares what
// the escaper rewrote with how the template parser, which follows the
// HTML tokenizer, reads them:
//
//   - a <!-- in <title> or <textarea> content, which html/template
//     escapes there, so browsers show it;
//   - a <! or <? html/template escapes, so browsers show it as text;
//   - a comment browsers end elsewhere than html/template does: at an
//     abrupt <!--> or <!--->, at --!>, or at a --> html/template strips
//     past because the comment runs into a {{template}} call;
//   - a comment, <! or <? in <xmp> or <plaintext> content, which browsers
//     show as raw text and html/template reads as HTML;
//   - the error the escaper stops with, which html/template returns
//     instead of rendering, for each root: the templates the program
//     executes by name. With no roots there is none, since a template
//     may be a fragment that is only ever called.
//
// A template is checked as html/template escapes it where it renders as
// text: as a root, in the element a {{template}} call in text is in, or,
// when nothing calls it, at the top level. One only called elsewhere, in
// a <script>, <style> or attribute, is escaped there as a copy and not
// checked; one called in <title> or <textarea> is checked for <!--.
//
// templates maps the name of each template in the set to its tree, parsed
// as HTML. CheckComments fails when html/template cannot parse a source.
func CheckComments(templates map[string]*template.Tree, roots []string) ([]TreeDiagnostic, error) {
	o := &escapeOracle{
		set:      htmltemplate.New(""),
		original: make(map[*parse.TextNode]string),
		stdlib:   make(map[string]*stdlibTemplate),
		ran:      make(map[string]bool),
		outers:   make(map[string]map[string]bool),
		derived:  make(map[string]bool),
	}
	if err := o.parse(templates); err != nil {
		return nil, err
	}
	names := slices.Sorted(maps.Keys(o.stdlib))
	for _, name := range roots {
		o.refusal(name)
	}
	// Roots first: html/template escapes a template once, in the context
	// of the first execution that reaches it, and a call in another
	// context escapes a copy.
	visited := make(map[[2]string]bool)
	for _, name := range roots {
		o.run(name)
		o.visit(name, "", visited)
	}
	// Then the templates nothing reached in text: first those no template
	// calls, then any left, as in a cycle of calls.
	referenced := make(map[string]bool)
	for _, st := range o.stdlib {
		forEachTemplateNode(st.parsed.Root, func(n *parse.TemplateNode) { referenced[n.Name] = true })
	}
	for _, entryPoints := range []bool{true, false} {
		for _, name := range names {
			if o.outers[name] != nil || o.derived[name] || entryPoints && referenced[name] {
				continue
			}
			o.run(name)
			o.visit(name, "", visited)
		}
	}
	for _, name := range names {
		for _, outer := range slices.Sorted(maps.Keys(o.outers[name])) {
			o.compare(o.stdlib[name], outer)
		}
	}
	slices.SortFunc(o.diags, func(a, b TreeDiagnostic) int {
		return cmp.Or(
			strings.Compare(a.Tree.Filepath, b.Tree.Filepath),
			strings.Compare(a.Tree.ParseName, b.Tree.ParseName),
			cmp.Compare(a.Pos, b.Pos),
			strings.Compare(a.Message, b.Message),
		)
	})
	return slices.CompactFunc(o.diags, func(a, b TreeDiagnostic) bool {
		return a.Tree == b.Tree && a.Pos == b.Pos && a.Message == b.Message
	}), nil
}

// escapeOracle holds a set parsed twice: by the template parser, the
// browser's view, and by text/template/parse into a standard library
// html/template set, whose escaper rewrites its text nodes.
type escapeOracle struct {
	set *htmltemplate.Template
	// original is the text of each text node before escaping.
	original map[*parse.TextNode]string
	stdlib   map[string]*stdlibTemplate
	// ran are the templates executed in set.
	ran map[string]bool
	// outers are, for each template html/template escaped as text, the
	// elements it renders in: "" at the top level, or the element a call
	// in text is in.
	outers map[string]map[string]bool
	// derived are the templates a call outside text reaches, which the
	// escaper escaped as copies.
	derived map[string]bool
	diags   []TreeDiagnostic
}

type stdlibTemplate struct {
	name string
	// parsed is the tree as parsed, and tree the copy in the oracle's
	// set, which the escaper rewrites.
	parsed, tree *parse.Tree
	// muxt is the template parser's tree of the same template, and view
	// how a browser reads its source.
	muxt *template.Tree
	view *browserView
}

// sourceKey identifies the text one parse read: a file, or a string
// given to Parse.
type sourceKey struct{ parseName, filepath, source string }

func (o *escapeOracle) parse(templates map[string]*template.Tree) error {
	const rootName = "\x00root"
	parsed := make(map[sourceKey]map[string]*parse.Tree)
	views := make(map[sourceKey]*browserView)
	for _, name := range slices.Sorted(maps.Keys(templates)) {
		mt := templates[name]
		if mt == nil || mt.Root == nil {
			continue
		}
		key := sourceKey{parseName: mt.ParseName, filepath: mt.Filepath, source: mt.Source()}
		trees, ok := parsed[key]
		if !ok {
			pt := parse.New(rootName)
			pt.Mode = parse.SkipFuncCheck
			left, right := mt.Delims()
			trees = make(map[string]*parse.Tree)
			if _, err := pt.Parse(key.source, left, right, trees); err != nil {
				return fmt.Errorf("html/template cannot parse %s: %w", cmp.Or(key.filepath, key.parseName), err)
			}
			parsed[key] = trees
			views[key] = newBrowserView()
		}
		view := views[key]
		view.add(mt)
		treeName := mt.Name
		if mt.Root.Start() == 0 {
			// Only the root starts at the start of the text; a define
			// body starts after its clause.
			treeName = rootName
		}
		pt := trees[treeName]
		if pt == nil {
			continue
		}
		parsedTree := pt.Copy()
		parsedTree.Name = name
		pt = parsedTree.Copy()
		if _, err := o.set.AddParseTree(name, pt); err != nil {
			return fmt.Errorf("html/template cannot add %q: %w", name, err)
		}
		o.stdlib[name] = &stdlibTemplate{name: name, parsed: parsedTree, tree: pt, muxt: mt, view: view}
		forEachTextNode(pt.Root, func(n *parse.TextNode) {
			o.original[n] = string(n.Text)
		})
	}
	return nil
}

// refusal reports the error html/template's escaper stops with when the
// program executes template name: html/template escapes a template when
// it first executes it. An error with no node is reported at the start
// of the template it is in: the deepest template name calls that the
// escaper, escaping it alone, stops with the same error, or name.
func (o *escapeOracle) refusal(name string) {
	st := o.stdlib[name]
	escapeErr, ok := o.escapeAlone(name)
	if st == nil || !ok {
		return
	}
	if in := o.stdlib[baseName(escapeErr.Name)]; in != nil {
		st = in
	}
	pos := st.muxt.Root.Start()
	switch {
	case escapeErr.Node != nil:
		pos = lex.Pos(escapeErr.Node.Position())
	default:
		if in := o.refusingCallee(name, name, escapeErr, make(map[string]bool)); in != nil {
			st, pos = in, in.muxt.Root.Start()
		}
	}
	description := escapeErr.Description
	if escapeErr.ErrorCode == htmltemplate.ErrEndContext && o.endsInComment(st.name) {
		description = msgUnclosedComment
	}
	o.report(st.view, pos, DiagEscaperRefuses, msgEscaperRefuses, description)
}

// refusingCallee returns the deepest template name calls, other than
// root, that the escaper stops with want escaping it alone.
func (o *escapeOracle) refusingCallee(root, name string, want *htmltemplate.Error, visited map[string]bool) *stdlibTemplate {
	st := o.stdlib[name]
	if st == nil || visited[name] {
		return nil
	}
	visited[name] = true
	var found *stdlibTemplate
	forEachTemplateNode(st.parsed.Root, func(n *parse.TemplateNode) {
		if found == nil {
			found = o.refusingCallee(root, n.Name, want, visited)
		}
	})
	if found != nil || name == root {
		return found
	}
	if got, ok := o.escapeAlone(name); ok && got.ErrorCode == want.ErrorCode && got.Description == want.Description {
		return st
	}
	return nil
}

// escapeAlone escapes template name in a set of its own, as parsed, and
// returns the error the escaper stops with. A set of its own because the
// escaper takes the tree of a template it refuses out of its set, so a
// later caller's escaping would find it missing.
func (o *escapeOracle) escapeAlone(name string) (*htmltemplate.Error, bool) {
	set, ok := o.parsedSet()
	if !ok {
		return nil, false
	}
	escapeErr, ok := errors.AsType[*htmltemplate.Error](execute(set, name))
	if !ok || escapeErr.ErrorCode == htmltemplate.ErrNoSuchTemplate {
		return nil, false
	}
	return escapeErr, true
}

// endsInComment reports whether template name, which the escaper refuses
// for ending in a context other than text, ends in a comment: a "-->"
// after it ends the context and a ">", which would end a tag, does not.
func (o *escapeOracle) endsInComment(name string) bool {
	return o.endsInTextAfter(name, commentClose) && !o.endsInTextAfter(name, ">")
}

func (o *escapeOracle) endsInTextAfter(name, text string) bool {
	set, ok := o.parsedSet()
	if !ok {
		return false
	}
	const wrapper = "\x00wrapper"
	trees, err := parse.Parse(wrapper, `{{template `+strconv.Quote(name)+`}}`+text, "{{", "}}")
	if err != nil {
		return false
	}
	if _, err := set.AddParseTree(wrapper, trees[wrapper]); err != nil {
		return false
	}
	_, refused := errors.AsType[*htmltemplate.Error](execute(set, wrapper))
	return !refused
}

// parsedSet returns a new html/template set of the templates as parsed.
func (o *escapeOracle) parsedSet() (*htmltemplate.Template, bool) {
	set := htmltemplate.New("")
	for _, name := range slices.Sorted(maps.Keys(o.stdlib)) {
		if _, err := set.AddParseTree(name, o.stdlib[name].parsed.Copy()); err != nil {
			return nil, false // parse added the same trees to the oracle's set
		}
	}
	return set, true
}

// run escapes template name in the oracle's set.
func (o *escapeOracle) run(name string) {
	if o.stdlib[name] == nil || o.ran[name] {
		return
	}
	o.ran[name] = true
	_ = execute(o.set, name) // refusal reports the escaper's error
}

// execute executes template name of set with no data and a writer that
// fails, which escapes it and the templates it calls, since html/template
// escapes before it writes, and stops at the first write. text/template
// panics executing a call to a template whose tree the escaper took out
// of the set; the panic is returned.
func execute(set *htmltemplate.Template, name string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("executing %q: %v", name, r)
		}
	}()
	return set.ExecuteTemplate(failingWriter{}, name, nil)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrShortWrite }

// visit records that the escaped template name renders as text in
// element outer, and follows its calls: one in text renders the callee in
// the element the call is in, and one outside text reaches a copy.
func (o *escapeOracle) visit(name, outer string, visited map[[2]string]bool) {
	st := o.stdlib[name]
	if st == nil || visited[[2]string{name, outer}] {
		return
	}
	visited[[2]string{name, outer}] = true
	if o.outers[name] == nil {
		o.outers[name] = make(map[string]bool)
	}
	o.outers[name][outer] = true
	forEachTemplateNode(st.tree.Root, func(n *parse.TemplateNode) {
		base := baseName(n.Name)
		if base != n.Name {
			o.derived[base] = true
			if el := rcdataElement(n.Name); el != "" {
				o.rcdataCallee(base, el, make(map[string]bool))
			}
			return
		}
		el, ok := st.view.textElement(lex.Pos(n.Pos))
		if !ok || el == "" {
			el = outer
		}
		o.visit(base, el, visited)
	})
}

// rcdataElement returns the element of a template html/template derived
// for <title> or <textarea> content, or "".
func rcdataElement(derived string) string {
	_, context, _ := strings.Cut(derived, derivedTemplateSep)
	switch {
	case !strings.HasPrefix(context, "stateRCDATA"):
		return ""
	case strings.Contains(context, "elementTitle"):
		return "title"
	case strings.Contains(context, "elementTextarea"):
		return "textarea"
	}
	return ""
}

// rcdataCallee reports each <!-- template name writes inside element el,
// <title> or <textarea>, where html/template escapes every <, through the
// end tag of el, in the templates it calls too.
func (o *escapeOracle) rcdataCallee(name, el string, visited map[string]bool) bool {
	st := o.stdlib[name]
	if st == nil || visited[name] {
		return false
	}
	visited[name] = true
	return o.rcdataList(st, st.tree.Root, el, visited)
}

func (o *escapeOracle) rcdataList(st *stdlibTemplate, l *parse.ListNode, el string, visited map[string]bool) bool {
	if l == nil {
		return false
	}
	for _, n := range l.Nodes {
		switch n := n.(type) {
		case *parse.TextNode:
			text := o.original[n]
			end := strings.Index(strings.ToLower(text), "</"+el)
			if end >= 0 {
				text = text[:end]
			}
			for i := 0; ; {
				k := strings.Index(text[i:], commentOpen)
				if k < 0 {
					break
				}
				o.report(st.view, lex.Pos(n.Pos)+lex.Pos(i+k), DiagCommentInRCDATA, msgCommentInRCDATA, el)
				i += k + len(commentOpen)
			}
			if end >= 0 {
				return true
			}
		case *parse.TemplateNode:
			if o.rcdataCallee(baseName(n.Name), el, visited) {
				return true
			}
		case *parse.IfNode:
			if o.rcdataList(st, n.List, el, visited) || o.rcdataList(st, n.ElseList, el, visited) {
				return true
			}
		case *parse.RangeNode:
			if o.rcdataList(st, n.List, el, visited) || o.rcdataList(st, n.ElseList, el, visited) {
				return true
			}
		case *parse.WithNode:
			if o.rcdataList(st, n.List, el, visited) || o.rcdataList(st, n.ElseList, el, visited) {
				return true
			}
		}
	}
	return false
}

// compare reports how the escaper rewrote the text of st, which renders
// in element outer.
func (o *escapeOracle) compare(st *stdlibTemplate, outer string) {
	c := comparison{oracle: o, view: st.view, outer: outer}
	c.list(st.tree.Root, nil)
}

// comparison walks the text nodes of one template in execution order.
type comparison struct {
	oracle *escapeOracle
	view   *browserView
	// outer is the element the template renders in, "" at the top level.
	outer string
}

// openComment is an HTML comment html/template is stripping: it starts at
// pos in the text of view, in a template rendered in element outer, and
// has not ended.
type openComment struct {
	view  *browserView
	pos   lex.Pos
	outer string
}

// list compares the nodes of l. open is the comment html/template is
// stripping when l starts; list returns the one it strips when l ends.
func (c comparison) list(l *parse.ListNode, open *openComment) *openComment {
	if l == nil {
		return open
	}
	for _, n := range l.Nodes {
		open = c.node(n, open)
	}
	return open
}

func (c comparison) node(n parse.Node, open *openComment) *openComment {
	switch n := n.(type) {
	case *parse.TextNode:
		return c.text(n, open)
	case *parse.IfNode:
		return c.branch(&n.BranchNode, open)
	case *parse.RangeNode:
		return c.branch(&n.BranchNode, open)
	case *parse.WithNode:
		return c.branch(&n.BranchNode, open)
	case *parse.TemplateNode:
		if open == nil || !strings.HasSuffix(n.Name, derivedForComment) {
			return open
		}
		// The escaper derived this callee for a comment: html/template
		// strips through the first --> it writes.
		callee := c.oracle.stdlib[baseName(n.Name)]
		if callee == nil {
			return open
		}
		if at, ok := c.oracle.commentEnd(callee, callee.tree.Root, make(map[string]bool)); ok {
			c.oracle.endComment(open, at.view, at.pos)
			return nil
		}
	}
	return open
}

// branch compares both branches of b; the escaper requires them to end
// in the same context.
func (c comparison) branch(b *parse.BranchNode, open *openComment) *openComment {
	end := c.list(b.List, open)
	c.list(b.ElseList, open)
	return end
}

// text compares text node n with what the escaper wrote for it.
func (c comparison) text(n *parse.TextNode, open *openComment) *openComment {
	original, ok := c.oracle.original[n]
	if !ok {
		return open
	}
	at := lex.Pos(n.Pos)
	from := 0
	if open != nil {
		i := strings.Index(original, commentClose)
		if i < 0 {
			return open
		}
		c.oracle.endComment(open, c.view, at+lex.Pos(i))
		open = nil
		from = i + len(commentClose)
	}
	if original == string(n.Text) {
		return open
	}
	edits, ok := textEdits(original, string(n.Text))
	if !ok {
		return open
	}
	for _, e := range edits {
		if e.start < from {
			continue
		}
		pos := at + lex.Pos(e.start)
		switch {
		case e.kind == escapedLT:
			c.oracle.escapedLT(c.view, pos, original[e.start:], c.outer)
		case !strings.HasPrefix(original[e.start:], commentOpen):
			// A CSS or JavaScript comment.
		case e.closed:
			c.oracle.endComment(&openComment{view: c.view, pos: pos, outer: c.outer}, c.view, at+lex.Pos(e.end-len(commentClose)))
		default:
			open = &openComment{view: c.view, pos: pos, outer: c.outer}
		}
	}
	return open
}

// endedAt is a position in the text of view.
type endedAt struct {
	view *browserView
	pos  lex.Pos
}

// commentEnd finds where html/template ends a comment open when the list
// l of template st starts: at the first --> in its text, in execution
// order, in the templates it calls too.
func (o *escapeOracle) commentEnd(st *stdlibTemplate, l *parse.ListNode, visited map[string]bool) (endedAt, bool) {
	if l == nil {
		return endedAt{}, false
	}
	for _, n := range l.Nodes {
		var branch *parse.BranchNode
		switch n := n.(type) {
		case *parse.TextNode:
			if i := strings.Index(o.original[n], commentClose); i >= 0 {
				return endedAt{view: st.view, pos: lex.Pos(n.Pos) + lex.Pos(i)}, true
			}
		case *parse.IfNode:
			branch = &n.BranchNode
		case *parse.RangeNode:
			branch = &n.BranchNode
		case *parse.WithNode:
			branch = &n.BranchNode
		case *parse.TemplateNode:
			name := baseName(n.Name)
			callee := o.stdlib[name]
			if callee == nil || visited[name] {
				continue
			}
			visited[name] = true
			if at, ok := o.commentEnd(callee, callee.tree.Root, visited); ok {
				return at, true
			}
		}
		if branch == nil {
			continue
		}
		if at, ok := o.commentEnd(st, branch.List, visited); ok {
			return at, true
		}
		if at, ok := o.commentEnd(st, branch.ElseList, visited); ok {
			return at, true
		}
	}
	return endedAt{}, false
}

// endComment compares where browsers end the comment open with where
// html/template does: at the --> at end in the text of endView.
func (o *escapeOracle) endComment(open *openComment, endView *browserView, end lex.Pos) {
	v := open.view
	comment, ok := v.comments[open.pos]
	if !ok {
		return
	}
	switch el := renderedIn(comment.element, open.outer); {
	case rawTextShown[el]:
		o.report(v, open.pos, DiagCommentInRawText, msgCommentInRawText, el)
		return
	case rawTextHidden[el], el == "script", el == "style":
		return
	}
	token, at := browserCommentEnd(comment.node)
	if token == commentClose && endView == v && at == end {
		return
	}
	o.report(v, open.pos, DiagCommentDelimiter, msgCommentDelimiter, token, v.position(at, v), endView.position(end, v))
}

// browserCommentEnd returns the token browsers end comment n at, and its
// position: an abrupt <!--> or <!--->, the first --!> in its text, or its
// -->.
func browserCommentEnd(n *template.HTMLCommentNode) (string, lex.Pos) {
	if src := n.String(); src == "<!-->" || src == "<!--->" {
		return src, n.Start()
	}
	for _, child := range n.Children {
		text, ok := child.(*template.TextNode)
		if !ok {
			continue
		}
		if i := strings.Index(string(text.Text), "--!>"); i >= 0 {
			return "--!>", text.Start() + lex.Pos(i)
		}
	}
	return commentClose, n.End() - lex.Pos(len(commentClose))
}

// escapedLT reports a < html/template writes as &lt; at pos in the text
// of v, in a template rendered in element outer, where browsers would not
// have shown the source as that text; text is the source from the <.
func (o *escapeOracle) escapedLT(v *browserView, pos lex.Pos, text, outer string) {
	el, ok := v.textElement(pos)
	if !ok || len(text) < len("<!") {
		return
	}
	el = renderedIn(el, outer)
	markup := text[:len("<!")]
	bogus := markup == "<!" || markup == "<?"
	switch {
	case el == "title" || el == "textarea":
		if strings.HasPrefix(text, commentOpen) {
			o.report(v, pos, DiagCommentInRCDATA, msgCommentInRCDATA, el)
		}
	case el == "script" || el == "style" || rawTextHidden[el] || !bogus:
	case rawTextShown[el]:
		o.report(v, pos, DiagEscapedInRawText, msgEscapedInRawText, markup, el)
	default:
		o.report(v, pos, DiagEscapedMarkup, msgEscapedMarkup, markup)
	}
}

func (o *escapeOracle) report(v *browserView, pos lex.Pos, code template.DiagnosticCode, format string, args ...any) {
	o.diags = append(o.diags, TreeDiagnostic{Tree: v.tree, Diagnostic: Diagnostic{
		Pos:      pos,
		Severity: SeverityError,
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
	}})
}

// renderedIn returns element el, or outer, the element its template renders
// in, when el is the template's top level.
func renderedIn(el, outer string) string {
	return cmp.Or(el, outer)
}

func forEachTemplateNode(l *parse.ListNode, fn func(*parse.TemplateNode)) {
	if l == nil {
		return
	}
	for _, n := range l.Nodes {
		switch n := n.(type) {
		case *parse.TemplateNode:
			fn(n)
		case *parse.IfNode:
			forEachTemplateNode(n.List, fn)
			forEachTemplateNode(n.ElseList, fn)
		case *parse.RangeNode:
			forEachTemplateNode(n.List, fn)
			forEachTemplateNode(n.ElseList, fn)
		case *parse.WithNode:
			forEachTemplateNode(n.List, fn)
			forEachTemplateNode(n.ElseList, fn)
		}
	}
}

// baseName is the name of the template html/template derived name from.
func baseName(name string) string {
	base, _, _ := strings.Cut(name, derivedTemplateSep)
	return base
}

func forEachTextNode(l *parse.ListNode, fn func(*parse.TextNode)) {
	if l == nil {
		return
	}
	for _, n := range l.Nodes {
		switch n := n.(type) {
		case *parse.TextNode:
			fn(n)
		case *parse.IfNode:
			forEachTextNode(n.List, fn)
			forEachTextNode(n.ElseList, fn)
		case *parse.RangeNode:
			forEachTextNode(n.List, fn)
			forEachTextNode(n.ElseList, fn)
		case *parse.WithNode:
			forEachTextNode(n.List, fn)
			forEachTextNode(n.ElseList, fn)
		}
	}
}

// browserView is how the template parser, which follows the HTML
// tokenizer, reads one source: the element each run of text is in, and
// the comments.
type browserView struct {
	// tree is a tree parsed from the source, to report positions in.
	tree     *template.Tree
	texts    []browserText
	comments map[lex.Pos]browserComment
	added    map[*template.Tree]bool
}

type browserText struct {
	start, end lex.Pos
	// element is the lower-case name of the element the text is in, ""
	// at the top level.
	element string
	// markup is false for text in an attribute or a comment.
	markup bool
}

type browserComment struct {
	node    *template.HTMLCommentNode
	element string
}

func newBrowserView() *browserView {
	return &browserView{comments: make(map[lex.Pos]browserComment), added: make(map[*template.Tree]bool)}
}

func (v *browserView) add(t *template.Tree) {
	if v.added[t] {
		return
	}
	v.added[t] = true
	if v.tree == nil || t.Root.Start() < v.tree.Root.Start() {
		v.tree = t
	}
	template.WalkPath(t, func(_ *template.Tree, path []template.Node, n template.Node) {
		element, markup := pathContext(path)
		switch n := n.(type) {
		case *template.TextNode, *template.HTMLDoctypeNode, *template.TemplateNode:
			v.texts = append(v.texts, browserText{start: n.Start(), end: n.End(), element: element, markup: markup})
		case *template.HTMLCommentNode:
			if markup {
				v.comments[n.Start()] = browserComment{node: n, element: element}
			}
		}
	})
	sort.SliceStable(v.texts, func(i, j int) bool { return v.texts[i].start < v.texts[j].start })
}

// pathContext returns the element a node with ancestors path is in, in
// its template, and whether it is in markup: not in an attribute or a
// comment.
func pathContext(path []template.Node) (string, bool) {
	element, markup := "", true
	for _, n := range path {
		switch n := n.(type) {
		case *template.DefineNode, *template.BlockNode:
			element, markup = "", true
		case *template.HTMLElementNode:
			element = strings.ToLower(n.TagName)
		case *template.HTMLAttributeNode, *template.HTMLCommentNode:
			markup = false
		}
	}
	return element, markup
}

// textElement returns the element the text at pos is in, when the parser
// read it as text outside attributes and comments.
func (v *browserView) textElement(pos lex.Pos) (string, bool) {
	i := sort.Search(len(v.texts), func(i int) bool { return v.texts[i].start > pos })
	if i == 0 {
		return "", false
	}
	t := v.texts[i-1]
	if pos >= t.end || !t.markup {
		return "", false
	}
	return t.element, true
}

// position formats pos in the text of v as line:column, prefixed with the
// file when v is not the text of from.
func (v *browserView) position(pos lex.Pos, from *browserView) string {
	p := v.tree.PositionAt(pos)
	if v == from {
		return fmt.Sprintf("%d:%d", p.Line, p.Column)
	}
	return fmt.Sprintf("%s:%d:%d", cmp.Or(v.tree.Filepath, p.ParseName), p.Line, p.Column)
}
