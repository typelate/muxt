package analysis

import (
	"cmp"
	"go/token"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/selectors"
)

// Display is the class of an element's outer display type: only the
// distinctions that matter to whitespace.
type Display uint8

const (
	DisplayInline        Display = iota // the CSS initial value
	DisplayBlock                        // block, list-item, flow-root, table, flex, grid, table-caption, …
	DisplayTableInternal                // table-row-group … table-cell, table-column(-group)
	DisplayAtomic                       // inline-block, inline-table, inline-flex, inline-grid, replaced elements
	DisplayNewline                      // br
	DisplayNone
	DisplayContents
	DisplayUnknown
)

func (d Display) String() string {
	switch d {
	case DisplayInline:
		return "inline"
	case DisplayBlock:
		return "block"
	case DisplayTableInternal:
		return "table-internal"
	case DisplayAtomic:
		return "atomic"
	case DisplayNewline:
		return "newline"
	case DisplayNone:
		return "none"
	case DisplayContents:
		return "contents"
	}
	return "unknown"
}

// WhiteSpace is the class of an element's white-space handling.
type WhiteSpace uint8

const (
	WhiteSpaceCollapse       WhiteSpace = iota // normal, nowrap
	WhiteSpacePreserveBreaks                   // pre-line
	WhiteSpacePreserve                         // pre, pre-wrap, break-spaces, white-space-collapse: preserve|break-spaces|preserve-spaces; raw text and RCDATA
	WhiteSpaceUnknown

	// whiteSpaceInherit is a declared value that takes the parent's.
	whiteSpaceInherit
)

func (w WhiteSpace) String() string {
	switch w {
	case WhiteSpaceCollapse:
		return "collapse"
	case WhiteSpacePreserveBreaks:
		return "preserve-breaks"
	case WhiteSpacePreserve:
		return "preserve"
	case whiteSpaceInherit:
		return "inherit"
	}
	return "unknown"
}

// Whitespace is what an element's whitespace handling depends on.
// WhiteSpace is the computed (inherited) value.
type Whitespace struct {
	Display    Display
	WhiteSpace WhiteSpace
}

// WhitespaceOptions is the author CSS ComputeWhitespace sees besides the
// <style> elements and style attributes of the trees it classifies.
type WhitespaceOptions struct {
	// Context holds templates whose <style> elements apply but whose
	// elements are not classified.
	Context []*template.Tree
	// Sheets are stylesheet files the templates use.
	Sheets []Stylesheet
	// AuthorCSSKnown reports that Sheets and the <style> elements are all
	// the author CSS (a package was loaded, or CSS files were named), even
	// when there is none. Without it, and with no <style> element or sheet,
	// an element with a class or id is unknown.
	AuthorCSSKnown bool
	// MissingSheets reports a stylesheet the templates use that could not
	// be loaded: author CSS may set anything on any element.
	MissingSheets bool
}

// Whitespaces maps the elements of templates to their whitespace handling.
type Whitespaces map[*template.HTMLElementNode]Whitespace

// ComputeWhitespace classifies every element of trees. Author CSS is the
// <style> elements and style attributes of trees and options.Context, plus
// options.Sheets. With options.MissingSheets, author CSS may also set
// anything on any element.
//
// The cascade has two origins: the HTML Standard's user-agent style sheet
// (ua.css) and the author CSS. A rule that may or may not apply (its
// selector depends on template data, it sits in @media, @supports,
// @container or @layer, or in a template branch) makes a property unknown
// when it could beat the rule that certainly applies with a different
// value. An element that floats or is absolutely positioned (float other
// than none, position absolute or fixed) and is not display: none has
// unknown display, since it is out of the inline flow. An element whose
// class or style attribute is dynamic is unknown, and so is one with a
// class or id when no author CSS is visible at all.
// White-space inherits; preserve and unknown white-space inherit to every
// descendant whatever its own CSS says. Define and block bodies inherit
// collapse.
func ComputeWhitespace(trees []*template.Tree, options WhitespaceOptions) Whitespaces {
	c := cascade{rules: sheetCSS{css: cssText{text: uaCSS}}.rules(false, 0, nil)}
	authors := authorSheets(slices.Concat(trees, options.Context), options.Sheets)
	c.authorVisible = options.AuthorCSSKnown || len(authors) > 0
	var filenames []string
	for _, sheet := range options.Sheets {
		filenames = append(filenames, sheet.Filename)
	}
	for i, sheet := range authors {
		c.rules = append(c.rules, sheet.rules(true, i+1, filenames)...)
	}
	if options.MissingSheets {
		c.rules = append(c.rules, cssRule{anyElement: true, maybe: true, author: true, sheet: len(authors) + 1, declarations: unknownDeclarations()})
	}
	out := make(Whitespaces)
	for _, root := range templateRoots(trees) {
		c.walk(root, WhiteSpaceCollapse, out)
	}
	return out
}

// cascade holds the style rules of both origins.
type cascade struct {
	rules         []cssRule
	authorVisible bool
}

func (c *cascade) walk(n template.Node, parent WhiteSpace, out Whitespaces) {
	switch n := n.(type) {
	case *template.HTMLElementNode:
		ws := c.classify(n, parent)
		out[n] = ws
		c.walkEach(n.Children, ws.WhiteSpace, out)
	case *template.HTMLDocumentFragmentNode:
		c.walkEach(n.Children, parent, out)
	case *template.ListNode:
		if n != nil {
			c.walkEach(n.Nodes, parent, out)
		}
	case *template.IfNode:
		c.walk(n.List, parent, out)
		c.walk(n.ElseList, parent, out)
	case *template.RangeNode:
		c.walk(n.List, parent, out)
		c.walk(n.ElseList, parent, out)
	case *template.WithNode:
		c.walk(n.List, parent, out)
		c.walk(n.ElseList, parent, out)
	}
}

func (c *cascade) walkEach(nodes []template.Node, parent WhiteSpace, out Whitespaces) {
	for _, n := range nodes {
		c.walk(n, parent, out)
	}
}

// rawTextElements hold content the HTML parser does not parse as markup;
// noscript is one when scripting is on.
var rawTextElements = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true, "xmp": true, "iframe": true,
	"noembed": true, "noframes": true, "plaintext": true, "noscript": true,
}

// replacedElements are atomic inline boxes when the cascade leaves them
// inline.
var replacedElements = map[string]bool{
	"img": true, "video": true, "audio": true, "iframe": true, "embed": true, "object": true,
	"canvas": true, "input": true, "select": true, "textarea": true, "meter": true, "progress": true,
}

func (c *cascade) classify(el *template.HTMLElementNode, parent WhiteSpace) Whitespace {
	tag := strings.ToLower(el.TagName)
	attribute, opaque := c.attributes(el)
	ws := Whitespace{Display: DisplayUnknown, WhiteSpace: WhiteSpaceUnknown}
	var spaces []WhiteSpace
	if !opaque {
		var displays []Display
		displays, spaces = c.candidates(el, attribute)
		ws.Display = oneDisplay(displays, replacedElements[tag])
	}
	switch {
	case parent == WhiteSpacePreserve || parent == WhiteSpaceUnknown:
		ws.WhiteSpace = parent
	case rawTextElements[tag]:
		ws.WhiteSpace = WhiteSpacePreserve
	case !opaque:
		ws.WhiteSpace = oneWhiteSpace(spaces, parent)
	}
	return ws
}

func oneDisplay(candidates []Display, replaced bool) Display {
	result := DisplayUnknown
	for i, d := range candidates {
		if d == DisplayInline && replaced {
			d = DisplayAtomic
		}
		if d == DisplayUnknown || (i > 0 && d != result) {
			return DisplayUnknown
		}
		result = d
	}
	return result
}

func oneWhiteSpace(candidates []WhiteSpace, parent WhiteSpace) WhiteSpace {
	result := WhiteSpaceUnknown
	for i, w := range candidates {
		if w == whiteSpaceInherit {
			w = parent
		}
		if w == WhiteSpaceUnknown || (i > 0 && w != result) {
			return WhiteSpaceUnknown
		}
		result = w
	}
	return result
}

// attributes returns the declarations of el's style attribute, and reports
// whether el is opaque: its attributes may change its style in ways the
// cascade cannot follow.
func (c *cascade) attributes(el *template.HTMLElementNode) ([]cssDeclaration, bool) {
	var (
		style []cssDeclaration
		named bool // el has a class or id
	)
	for _, n := range el.Attr {
		switch n := n.(type) {
		case *template.HTMLAttributeNode:
			if n.Namespace != "" {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(staticAttrName(n)))
			switch name {
			case "":
				return nil, true
			case "class":
				if !isStatic(n.Value) {
					return nil, true
				}
				named = named || attributeHasText(n)
			case "id":
				named = true
			case "style":
				decls, ok := styleAttributeDeclarations(n)
				if !ok {
					return nil, true
				}
				style = decls
			}
		case *template.IfNode, *template.RangeNode, *template.WithNode:
			id, ok := conditionalAttributes(n)
			if !ok {
				return nil, true
			}
			named = named || id
		case *template.CommentNode:
		default:
			return nil, true
		}
	}
	if named && !c.authorVisible {
		return nil, true
	}
	return style, false
}

// conditionalAttributes reports whether the attributes in the branches of n
// leave the element's style to the cascade (they may not add a class, a
// style or hidden), and whether they may add an id.
func conditionalAttributes(n template.Node) (id, ok bool) {
	var lists []*template.ListNode
	switch n := n.(type) {
	case *template.IfNode:
		lists = []*template.ListNode{n.List, n.ElseList}
	case *template.RangeNode:
		lists = []*template.ListNode{n.List, n.ElseList}
	case *template.WithNode:
		lists = []*template.ListNode{n.List, n.ElseList}
	}
	for _, list := range lists {
		if list == nil {
			continue
		}
		for _, item := range list.Nodes {
			switch item := item.(type) {
			case *template.HTMLAttributeNode:
				switch strings.ToLower(strings.TrimSpace(staticAttrName(item))) {
				case "", "class", "style", "hidden":
					return false, false
				case "id":
					id = true
				}
			case *template.IfNode, *template.RangeNode, *template.WithNode:
				nestedID, nestedOK := conditionalAttributes(item)
				if !nestedOK {
					return false, false
				}
				id = id || nestedID
			case *template.TextNode, *template.CommentNode:
			default:
				return false, false
			}
		}
	}
	return id, true
}

func isStatic(nodes []template.Node) bool {
	for _, n := range nodes {
		if _, ok := n.(*template.TextNode); !ok {
			return false
		}
	}
	return true
}

// attributeHasText reports whether the static value of attr has a character
// other than whitespace and quotes.
func attributeHasText(attr *template.HTMLAttributeNode) bool {
	for _, n := range attr.Value {
		if t, ok := n.(*template.TextNode); ok && strings.Trim(string(t.Text), " \t\n\r\f\"'") != "" {
			return true
		}
	}
	return false
}

// styleAttributeDeclarations returns the display and white-space
// declarations of a style attribute. It reports false when a template
// action may change them: an action anywhere but in the value of a
// declaration of another property.
func styleAttributeDeclarations(attr *template.HTMLAttributeNode) ([]cssDeclaration, bool) {
	if len(attr.Value) == 0 {
		return nil, true
	}
	if slices.ContainsFunc(attr.Value, isBranch) {
		return nil, false // its declarations may or may not be there
	}
	css := templateCSS(attr.Value)
	css.unquote()
	ast, err := csstree.Parse(css.text, csstree.ParseOptions{Context: "declarationList", Positions: true})
	if err != nil {
		return nil, false
	}
	list, ok := ast.(*csstree.DeclarationList)
	if !ok {
		return nil, false
	}
	var (
		decls  []cssDeclaration
		exempt []span
	)
	for item := list.Children.Head(); item != nil; item = item.Next {
		decl, ok := item.Data.(*csstree.Declaration)
		if !ok || decl.Loc == nil {
			continue
		}
		start, end := decl.Loc.Start.Offset, declarationEnd(css.text, decl, item.Next, len(css.text))
		colon := strings.IndexByte(css.text[start:end], ':')
		if colon < 0 {
			continue
		}
		colon += start
		prop := strings.ToLower(decl.Property)
		if !trackedProperties[prop] && !css.dynamic(start, colon) {
			exempt = append(exempt, span{colon + 1, end})
			continue
		}
		decls = append(decls, propertyDeclarations(prop, valueWords(decl), decl.Important != "", true, len(decls))...)
	}
	for _, gap := range css.gaps {
		if !within(gap, exempt) {
			return nil, false
		}
	}
	return decls, true
}

// trackedProperties are the properties whose values whitespace depends on.
// text-wrap-mode does not change the class, but an action that may set it
// may set white-space too.
var trackedProperties = map[string]bool{
	"display": true, "display-outside": true,
	"white-space": true, "white-space-collapse": true, "text-wrap-mode": true,
	"float": true, "position": true,
	"all": true,
}

func within(s span, spans []span) bool {
	for _, o := range spans {
		if o.start <= s.start && s.end <= o.end {
			return true
		}
	}
	return false
}

// candidates returns the display and white-space values el may take: the
// winner of the rules that certainly apply, then the values of the rules
// that may apply and could beat it.
func (c *cascade) candidates(el *template.HTMLElementNode, attribute []cssDeclaration) ([]Display, []WhiteSpace) {
	var applied [propertyCount][]application
	add := func(d cssDeclaration, r rank, certain bool) {
		applied[d.property] = append(applied[d.property], application{rank: r, certain: certain, value: d.value})
	}
	for _, rule := range c.rules {
		if rule.anyElement {
			for _, d := range rule.declarations {
				add(d, rule.rank(d, Specificity{}), false)
			}
			continue
		}
		for _, sel := range rule.selectors {
			ok, trit := sel.selector.Matches(el)
			if !ok {
				continue
			}
			for _, d := range rule.declarations {
				add(d, rule.rank(d, sel.specificity), trit == selectors.TritYes && !rule.maybe && !d.maybe)
			}
		}
	}
	for _, d := range attribute {
		add(d, rank{level: cascadeLevel(true, d.important), attribute: true, order: d.order}, true)
	}
	var (
		displays []Display
		spaces   []WhiteSpace
	)
	for _, v := range cascadeValues(applied[propertyDisplay], uint8(DisplayInline)) {
		displays = append(displays, Display(v))
	}
	// A float or an absolutely positioned box is out of flow: it is
	// blockified and taken out of the inline formatting context, which
	// none of the display classes describes. An element that is certainly
	// display: none has no box to take out of flow.
	hidden := !slices.ContainsFunc(displays, func(d Display) bool { return d != DisplayNone })
	for _, property := range []int{propertyFloat, propertyPosition} {
		if hidden {
			break
		}
		for _, v := range cascadeValues(applied[property], flowIn) {
			if v != flowIn {
				displays = append(displays, DisplayUnknown)
			}
		}
	}
	for _, v := range cascadeValues(applied[propertyWhiteSpace], uint8(whiteSpaceInherit)) {
		spaces = append(spaces, WhiteSpace(v))
	}
	return displays, spaces
}

// application is a declaration that applies, or may apply, to an element.
type application struct {
	rank    rank
	certain bool
	value   uint8 // a Display or a WhiteSpace
}

// cascadeValues returns the value of the certain application that wins, or
// initial when none applies, followed by the values of the applications
// that could beat it.
func cascadeValues(applied []application, initial uint8) []uint8 {
	winner := -1
	for i, a := range applied {
		if a.certain && (winner < 0 || a.rank.beats(applied[winner].rank)) {
			winner = i
		}
	}
	values := []uint8{initial}
	if winner >= 0 {
		values[0] = applied[winner].value
	}
	for i, a := range applied {
		if i != winner && (winner < 0 || a.rank.mayBeat(applied[winner].rank)) {
			values = append(values, a.value)
		}
	}
	return values
}

const (
	levelUA = iota
	levelAuthor
	levelAuthorImportant
	levelUAImportant
)

func cascadeLevel(author, important bool) int {
	switch {
	case author && important:
		return levelAuthorImportant
	case author:
		return levelAuthor
	case important:
		return levelUAImportant
	}
	return levelUA
}

// rank is a declaration's place in the cascade.
type rank struct {
	level       int
	attribute   bool // from a style attribute
	specificity Specificity
	sheet       int // the order of style sheets is not known
	order       int
	// unknown reports that only level is known: the declaration is in a
	// @layer, or its selector is unknown.
	unknown bool
}

// beats orders certain declarations.
func (r rank) beats(o rank) bool {
	switch {
	case r.level != o.level:
		return r.level > o.level
	case r.attribute != o.attribute:
		return r.attribute
	case r.specificity != o.specificity:
		return o.specificity.less(r.specificity)
	case r.sheet != o.sheet:
		return r.sheet > o.sheet
	}
	return r.order > o.order
}

// mayBeat reports whether r could beat o: it does, or what decides is not
// known.
func (r rank) mayBeat(o rank) bool {
	switch {
	case r.level != o.level:
		return r.level > o.level
	case r.unknown || o.unknown:
		return true
	case r.attribute != o.attribute:
		return r.attribute
	case r.specificity != o.specificity:
		return o.specificity.less(r.specificity)
	case r.sheet != o.sheet:
		return true
	}
	return r.order > o.order
}

const (
	propertyDisplay = iota
	propertyWhiteSpace
	propertyFloat
	propertyPosition
	propertyCount
)

// The values of float and position declarations.
const (
	flowIn      uint8 = iota // float: none; position: static, relative, sticky
	flowOut                  // any other float; position: absolute, fixed
	flowUnknown              // a value htt cannot read
)

// cssDeclaration is a declaration of display or white-space reduced to its
// class.
type cssDeclaration struct {
	property  int
	value     uint8 // a Display, a WhiteSpace, or a flow value (float, position)
	important bool
	maybe     bool // it is in a template branch
	order     int
}

// cssRule is a style rule's selectors and its display and white-space
// declarations.
type cssRule struct {
	selectors []ruleSelector
	// anyElement reports that the selectors are unknown, so the rule may
	// match any element.
	anyElement   bool
	maybe        bool // the rule applies only under conditions: @media, a template branch, …
	author       bool
	sheet        int
	rankUnknown  bool
	declarations []cssDeclaration
}

type ruleSelector struct {
	selector    selectors.Selector
	specificity Specificity
}

func (r cssRule) rank(d cssDeclaration, specificity Specificity) rank {
	return rank{
		level:       cascadeLevel(r.author, d.important),
		specificity: specificity,
		sheet:       r.sheet,
		order:       d.order,
		unknown:     r.rankUnknown || r.anyElement,
	}
}

// unknownDeclarations stand for CSS that may set anything, even
// !important.
func unknownDeclarations() []cssDeclaration {
	return []cssDeclaration{
		{property: propertyDisplay, value: uint8(DisplayUnknown), important: true},
		{property: propertyWhiteSpace, value: uint8(WhiteSpaceUnknown), important: true},
		{property: propertyFloat, value: flowUnknown, important: true},
		{property: propertyPosition, value: flowUnknown, important: true},
	}
}

// sheetCSS is the CSS of a <style> element, a stylesheet file or ua.css.
type sheetCSS struct {
	css cssText
	// url is the URL path of a stylesheet file, which its @import rules
	// resolve against; "" for a <style> element, whose resolve against "/".
	url string
	// conditional reports that the <style> element is in a template branch.
	conditional bool
	// outputs are the spans of template actions that print text; branches
	// are the spans of if, range and with actions.
	outputs, branches []span
}

// authorSheets returns the <style> elements of trees, each once even when
// several trees hold it, and sheets.
func authorSheets(trees []*template.Tree, sheets []Stylesheet) []sheetCSS {
	var out []sheetCSS
	seen := make(map[token.Position]bool)
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		filename := templateSource(t).filename
		template.WalkPath(t, func(_ *template.Tree, path []template.Node, n template.Node) {
			el, ok := n.(*template.HTMLElementNode)
			if !ok || !strings.EqualFold(el.TagName, "style") || len(el.Children) == 0 {
				return
			}
			css := templateCSS(el.Children)
			key := token.Position{Filename: filename, Offset: int(css.base)}
			if seen[key] {
				return
			}
			seen[key] = true
			sheet := sheetCSS{css: css, conditional: slices.ContainsFunc(path, isBranch)}
			for _, child := range el.Children {
				sheet.addActions(child)
			}
			out = append(out, sheet)
		})
	}
	for _, s := range sheets {
		out = append(out, sheetCSS{css: cssText{text: s.Text}, url: s.Filename, conditional: s.Conditional})
	}
	return out
}

func isBranch(n template.Node) bool {
	switch n.(type) {
	case *template.IfNode, *template.RangeNode, *template.WithNode:
		return true
	}
	return false
}

func (s *sheetCSS) addActions(n template.Node) {
	if l, ok := n.(*template.ListNode); ok && l == nil {
		return
	}
	at := span{int(n.Start() - s.css.base), int(n.End() - s.css.base)}
	switch n := n.(type) {
	case *template.TextNode, *template.CommentNode:
	case *template.ListNode:
		for _, c := range n.Nodes {
			s.addActions(c)
		}
	case *template.IfNode:
		s.branches = append(s.branches, at)
		s.addActions(n.List)
		s.addActions(n.ElseList)
	case *template.RangeNode:
		s.branches = append(s.branches, at)
		s.addActions(n.List)
		s.addActions(n.ElseList)
	case *template.WithNode:
		s.branches = append(s.branches, at)
		s.addActions(n.List)
		s.addActions(n.ElseList)
	default:
		s.outputs = append(s.outputs, at)
	}
}

// rules returns the display and white-space rules of s. CSS that does not
// parse, an @import of a sheet not among filenames, and a printing action
// whose output may be any CSS become a rule that may set anything on any
// element.
func (s sheetCSS) rules(author bool, sheet int, filenames []string) []cssRule {
	unknown := cssRule{anyElement: true, maybe: true, author: author, sheet: sheet, declarations: unknownDeclarations()}
	c := ruleCollector{css: s.css, url: cmp.Or(s.url, "/"), author: author, sheet: sheet, branches: s.branches, filenames: filenames}
	if !c.collect(ruleContext{maybe: s.conditional}) || c.incomplete {
		return append(c.rules, unknown)
	}
	for _, out := range s.outputs {
		if !within(out, c.covered) {
			return append(c.rules, unknown)
		}
	}
	return c.rules
}

// ruleCollector reduces a style sheet to its display and white-space
// rules.
type ruleCollector struct {
	css      cssText
	url      string // the URL path @import targets resolve against
	author   bool
	sheet    int
	branches []span
	order    int
	rules    []cssRule
	// covered are the spans where a printing action's effect is known:
	// selector lists, which become unknown, and declaration values.
	covered []span
	// filenames are the stylesheet files an @import may name.
	filenames []string
	// incomplete reports an @import of a sheet not among filenames.
	incomplete bool
}

// ruleContext is what the statements of a block are inside.
type ruleContext struct {
	nesting nestingContext
	// owner is the enclosing style rule's selectors, which bare
	// declarations apply to; nil at the top level.
	owner       []ruleSelector
	anyElement  bool
	maybe       bool
	rankUnknown bool
}

func (c *ruleCollector) collect(ctx ruleContext) bool {
	sheet, ok := parseStylesheet(c.css)
	if !ok {
		return false
	}
	c.statements(sheet.Children, len(c.css.text), ctx)
	return true
}

// statements collects the rules in list, and the declarations in it when
// it is the block of a style rule or of an at-rule nested in one.
func (c *ruleCollector) statements(list *csstree.List[csstree.Node], end int, ctx ruleContext) {
	if ctx.owner != nil || ctx.anyElement {
		rule := cssRule{
			selectors:   ctx.owner,
			anyElement:  ctx.anyElement,
			maybe:       ctx.maybe || ctx.anyElement,
			author:      c.author,
			sheet:       c.sheet,
			rankUnknown: ctx.rankUnknown,
		}
		c.declarations(list, end, &rule)
		if len(rule.declarations) > 0 {
			c.rules = append(c.rules, rule)
		}
	}
	for item := list.Head(); item != nil; item = item.Next {
		switch n := item.Data.(type) {
		case *csstree.Atrule:
			name := strings.ToLower(n.Name)
			if name == "import" && !c.imports(n) {
				c.incomplete = true
				continue
			}
			if n.Block == nil || !(groupingAtRules[name] || name == "scope") {
				continue
			}
			inner := ctx
			inner.maybe = true
			inner.rankUnknown = ctx.rankUnknown || name == "layer" || name == "scope"
			c.statements(n.Block.Children, blockEnd(c.css, n.Block), inner)
		case *csstree.Rule:
			c.rule(n, ctx)
		}
	}
}

func (c *ruleCollector) rule(rule *csstree.Rule, ctx ruleContext) {
	if rule.Block == nil || rule.Block.Loc == nil || rule.Loc == nil {
		return
	}
	c.covered = append(c.covered, span{rule.Loc.Start.Offset, rule.Block.Loc.Start.Offset})
	inner := ctx
	inner.owner = nil
	list, ok := rule.Prelude.(*csstree.SelectorList)
	if !ok || list.Loc == nil || c.css.dynamic(list.Loc.Start.Offset, preludeEnd(rule)) {
		inner.anyElement = true
	}
	var resolved []string
	inner.nesting = nestingContext{}
	var head *csstree.ListItem[csstree.Node]
	if !inner.anyElement {
		head = list.Children.Head()
	}
	for item := head; item != nil; item = item.Next {
		sel, ok := item.Data.(*csstree.Selector)
		if !ok {
			inner.anyElement = true
			break
		}
		if hasPseudoElement(sel) {
			continue // styles a box other than the element's
		}
		query, _, ok := resolveSelector(sel, ctx.nesting.query)
		if !ok {
			inner.anyElement = true
			break
		}
		compiled, err := selectors.Compile(query)
		if err != nil {
			inner.anyElement = true
			break
		}
		specificity := selectorSpecificity(sel, ctx.nesting.specificity)
		inner.owner = append(inner.owner, ruleSelector{selector: compiled, specificity: specificity})
		resolved = append(resolved, query)
		if inner.nesting.specificity.less(specificity) {
			inner.nesting.specificity = specificity
		}
	}
	if inner.anyElement {
		inner.owner = nil
		inner.rankUnknown = true
	} else if len(inner.owner) == 0 {
		return // only pseudo-elements
	}
	inner.nesting.query = strings.Join(resolved, ", ")
	c.statements(rule.Block.Children, blockEnd(c.css, rule.Block), inner)
}

// imports reports whether the sheet rule imports is among c.filenames:
// its target, resolved against the URL path of the importing sheet, is a
// sheet's Filename read as a URL path.
func (c *ruleCollector) imports(rule *csstree.Atrule) bool {
	ref := atImportTarget(rule)
	if ref == "" {
		return false
	}
	target, ok := resolveStylesheetURL(ref, c.url)
	if !ok {
		return false
	}
	for _, name := range c.filenames {
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		if name == target {
			return true
		}
	}
	return false
}

// importTarget returns the URL an @import prelude names, or "".
func importTarget(prelude string) string {
	prelude = strings.TrimSpace(prelude)
	if rest, ok := strings.CutPrefix(strings.ToLower(prelude), "url("); ok {
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			return ""
		}
		return strings.Trim(strings.TrimSpace(prelude[len("url("):len("url(")+end]), `"'`)
	}
	if prelude == "" || (prelude[0] != '"' && prelude[0] != '\'') {
		return ""
	}
	end := strings.IndexByte(prelude[1:], prelude[0])
	if end < 0 {
		return ""
	}
	return prelude[1 : 1+end]
}

// hasPseudoElement reports whether sel selects a pseudo-element.
func hasPseudoElement(sel *csstree.Selector) bool {
	for item := sel.Children.Head(); item != nil; item = item.Next {
		switch n := item.Data.(type) {
		case *csstree.PseudoElementSelector:
			return true
		case *csstree.PseudoClassSelector:
			switch strings.ToLower(n.Name) {
			case "before", "after", "first-line", "first-letter":
				return true
			}
		}
	}
	return false
}

// declarations adds the display and white-space declarations of list to
// rule.
func (c *ruleCollector) declarations(list *csstree.List[csstree.Node], end int, rule *cssRule) {
	for item := list.Head(); item != nil; item = item.Next {
		decl, ok := item.Data.(*csstree.Declaration)
		if !ok || decl.Loc == nil {
			continue
		}
		start, declEnd := decl.Loc.Start.Offset, declarationEnd(c.css.text, decl, item.Next, end)
		maybe := c.inBranch(start)
		colon := strings.IndexByte(c.css.text[start:declEnd], ':')
		if colon < 0 {
			continue
		}
		colon += start
		c.covered = append(c.covered, span{colon + 1, declEnd})
		var decls []cssDeclaration
		prop := strings.ToLower(decl.Property)
		switch {
		case c.css.dynamic(start, colon):
			decls = unknownDeclarations()
		case !trackedProperties[prop]:
			continue
		case c.css.dynamic(colon+1, declEnd):
			decls = propertyDeclarations(prop, nil, true, c.author, 0)
		default:
			decls = propertyDeclarations(prop, valueWords(decl), decl.Important != "", c.author, 0)
		}
		for _, d := range decls {
			d.maybe = maybe
			d.order = c.order
			c.order++
			rule.declarations = append(rule.declarations, d)
		}
	}
}

func (c *ruleCollector) inBranch(offset int) bool {
	for _, b := range c.branches {
		if b.start <= offset && offset < b.end {
			return true
		}
	}
	return false
}

// valueWords returns the lowercase words of decl's value, or nil when the
// value is not a list of keywords.
func valueWords(decl *csstree.Declaration) []string {
	if decl.Value == nil {
		return nil
	}
	text, err := csstree.Generate(decl.Value, csstree.GenerateOptions{})
	if err != nil || strings.ContainsAny(text, "(,/") {
		return nil
	}
	return strings.Fields(strings.ToLower(text))
}

// propertyDeclarations reduces a declaration of prop to the classes it
// sets. words nil means the value is unknown. Browsers do not implement
// display-outside, so only the UA sheet's (br, wbr) counts.
func propertyDeclarations(prop string, words []string, important, author bool, order int) []cssDeclaration {
	display := func(d Display) cssDeclaration {
		return cssDeclaration{property: propertyDisplay, value: uint8(d), important: important, order: order}
	}
	whiteSpace := func(w WhiteSpace) cssDeclaration {
		return cssDeclaration{property: propertyWhiteSpace, value: uint8(w), important: important, order: order}
	}
	flow := func(property int, v uint8) cssDeclaration {
		return cssDeclaration{property: property, value: v, important: important, order: order}
	}
	switch prop {
	case "display":
		return []cssDeclaration{display(displayValue(words))}
	case "display-outside":
		if author {
			return nil
		}
		return []cssDeclaration{display(displayOutsideValue(words))}
	case "white-space", "white-space-collapse":
		return []cssDeclaration{whiteSpace(whiteSpaceValue(words))}
	case "float":
		return []cssDeclaration{flow(propertyFloat, floatValue(words))}
	case "position":
		return []cssDeclaration{flow(propertyPosition, positionValue(words))}
	case "all":
		d, w, f := DisplayUnknown, WhiteSpaceUnknown, flowUnknown
		if len(words) == 1 {
			switch words[0] {
			case "initial", "unset":
				d, w, f = DisplayInline, WhiteSpaceCollapse, flowIn
				if words[0] == "unset" {
					w = whiteSpaceInherit
				}
			case "inherit":
				w = whiteSpaceInherit
			}
		}
		return []cssDeclaration{display(d), whiteSpace(w), flow(propertyFloat, f), flow(propertyPosition, f)}
	}
	return nil // text-wrap-mode does not change the class
}

func displayValue(words []string) Display {
	if len(words) == 0 {
		return DisplayUnknown
	}
	if len(words) == 1 {
		switch words[0] {
		case "none":
			return DisplayNone
		case "contents":
			return DisplayContents
		case "inline", "ruby", "run-in", "ruby-base", "ruby-text", "ruby-base-container", "ruby-text-container", "math", "initial", "unset":
			return DisplayInline
		case "block", "list-item", "flow-root", "flow", "table", "flex", "grid", "table-caption", "-webkit-box":
			return DisplayBlock
		case "inline-block", "inline-table", "inline-flex", "inline-grid", "-webkit-inline-box":
			return DisplayAtomic
		case "table-row-group", "table-header-group", "table-footer-group", "table-row", "table-cell", "table-column", "table-column-group":
			return DisplayTableInternal
		}
		return DisplayUnknown
	}
	var outside, inside string
	for _, w := range words {
		switch w {
		case "block", "inline", "run-in":
			outside = w
		case "flow", "flow-root", "table", "flex", "grid", "ruby":
			inside = w
		case "list-item":
		default:
			return DisplayUnknown
		}
	}
	switch outside {
	case "block":
		return DisplayBlock
	case "inline":
		if inside == "flow" || inside == "ruby" || inside == "" {
			return DisplayInline
		}
		return DisplayAtomic
	case "run-in":
		return DisplayInline
	}
	if inside == "ruby" {
		return DisplayInline
	}
	return DisplayBlock
}

func displayOutsideValue(words []string) Display {
	if len(words) != 1 {
		return DisplayUnknown
	}
	switch words[0] {
	case "newline":
		return DisplayNewline
	case "block":
		return DisplayBlock
	case "inline", "run-in", "break-opportunity", "initial", "unset":
		return DisplayInline
	}
	return DisplayUnknown
}

func whiteSpaceValue(words []string) WhiteSpace {
	if len(words) == 0 {
		return WhiteSpaceUnknown
	}
	if len(words) == 1 {
		switch words[0] {
		case "normal", "nowrap", "initial":
			return WhiteSpaceCollapse
		case "pre", "pre-wrap":
			return WhiteSpacePreserve
		case "pre-line":
			return WhiteSpacePreserveBreaks
		case "inherit", "unset":
			return whiteSpaceInherit
		}
	}
	result := WhiteSpaceCollapse
	for _, w := range words {
		switch w {
		case "preserve", "preserve-spaces", "break-spaces":
			result = WhiteSpacePreserve
		case "preserve-breaks":
			result = WhiteSpacePreserveBreaks
		case "collapse", "wrap", "nowrap":
		default:
			return WhiteSpaceUnknown
		}
	}
	return result
}

// floatValue classifies a float value. Inherit is unknown: float does not
// inherit, and the parent's value is not tracked.
func floatValue(words []string) uint8 {
	if len(words) != 1 {
		return flowUnknown
	}
	switch words[0] {
	case "none", "initial", "unset":
		return flowIn
	case "inherit":
		return flowUnknown
	}
	return flowOut // left, right, inline-start, …: every value but none floats
}

// positionValue classifies a position value.
func positionValue(words []string) uint8 {
	if len(words) != 1 {
		return flowUnknown
	}
	switch words[0] {
	case "static", "relative", "sticky", "initial", "unset":
		return flowIn
	case "absolute", "fixed":
		return flowOut
	}
	return flowUnknown
}
