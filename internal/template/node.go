package template

import (
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/typelate/muxt/internal/template/lex"
)

// A Node is an element in the parse tree. The interface is trivial.
// The interface contains an unexported method so that only
// types local to this package can satisfy it.
type Node interface {
	String() string
	Start() lex.Pos // byte position of start of node in full original input string
	End() lex.Pos   // byte position of end of node (exclusive)

	// tree returns the containing *Tree.
	// It is unexported so all implementations of Node are in this package.
	tree() *Tree

	// writeTo writes the String output to the builder.
	writeTo(*strings.Builder)
}

func (t *Tree) src(pos, end lex.Pos) string {
	if t == nil || int(end) > len(t.text) || pos >= end {
		return ""
	}
	return t.text[pos:end]
}

// ListNode holds a sequence of nodes.
type ListNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Nodes      []Node // The element nodes in lexical order.
}

func (l ListNode) Start() lex.Pos { return l.start }

func (l ListNode) End() lex.Pos { return l.end }
func (t *Tree) newList(pos lex.Pos) *ListNode {
	return &ListNode{tr: t, start: pos}
}

func (l ListNode) String() string { return l.tr.src(l.Start(), l.End()) }
func (l ListNode) tree() *Tree {
	return l.tr
}

func (l ListNode) writeTo(sb *strings.Builder) {
	for _, n := range l.Nodes {
		n.writeTo(sb)
	}
}

// TextNode holds plain text.
type TextNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Text       []byte // The text; may span newlines.
	count      atomic.Int64

	// Verbatim marks text that must render without HTML escaping in
	// HTML mode. The parser sets it for preserved orphan end tags,
	// which browsers ignore but stdlib html/template passes through.
	Verbatim bool
}

func (t *Tree) newText(start, end lex.Pos, text string) *TextNode {
	return &TextNode{tr: t, start: start, end: end, Text: []byte(text)}
}

func (t *TextNode) Start() lex.Pos              { return t.start }
func (t *TextNode) End() lex.Pos                { return t.end }
func (t *TextNode) String() string              { return t.tr.src(t.Start(), t.End()) }
func (t *TextNode) writeTo(sb *strings.Builder) { sb.WriteString(t.String()) }
func (t *TextNode) tree() *Tree                 { return t.tr }

// CommentNode holds a comment.
type CommentNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Text       string // Comment text.
	TrimLeft   bool   // {{- was used
	TrimRight  bool   // -}} was used
}

func (t *Tree) newComment(start, end lex.Pos, text string) *CommentNode {
	return &CommentNode{tr: t, start: start, end: end, Text: text}
}

func (c *CommentNode) Start() lex.Pos { return c.start }
func (c *CommentNode) End() lex.Pos   { return c.end }
func (c *CommentNode) String() string { return c.tr.src(c.Start(), c.End()) }

func (c *CommentNode) tree() *Tree { return c.tr }

func (c *CommentNode) writeTo(sb *strings.Builder) {
	sb.WriteString("{{")
	sb.WriteString(c.Text)
	sb.WriteString("}}")
}

// PipeNode holds a pipeline with optional declaration
type PipeNode struct {
	tr         *Tree
	start, end lex.Pos
	Line       int             // The line number in the input. Deprecated: Kept for compatibility.
	IsAssign   bool            // The variables are being assigned, not declared.
	Decl       []*VariableNode // Variables in lexical order.
	Commands   []*CommandNode  // The commands in lexical order.
}

func (t *Tree) newPipeline(pos lex.Pos, line int, vars []*VariableNode) *PipeNode {
	return &PipeNode{tr: t, start: pos, Line: line, Decl: vars}
}

func (p *PipeNode) Start() lex.Pos { return p.start }
func (p *PipeNode) End() lex.Pos   { return p.end }
func (p *PipeNode) String() string { return p.tr.src(p.start, p.End()) }
func (p *PipeNode) tree() *Tree    { return p.tr }

func (p *PipeNode) writeTo(sb *strings.Builder) {
	if len(p.Decl) > 0 {
		for i, v := range p.Decl {
			if i > 0 {
				sb.WriteString(", ")
			}
			v.writeTo(sb)
		}
		if p.IsAssign {
			sb.WriteString(" = ")
		} else {
			sb.WriteString(" := ")
		}
	}
	for i, c := range p.Commands {
		if i > 0 {
			sb.WriteString(" | ")
		}
		c.writeTo(sb)
	}
}

// ActionNode holds an action (something bounded by delimiters).
// Control actions have their own nodes; ActionNode represents simple
// ones such as field evaluations and parenthesized pipelines.
type ActionNode struct {
	nodeParent
	tr          *Tree
	start, end  lex.Pos
	Line        int          // The line number in the input. Deprecated: Kept for compatibility.
	Pipe        *PipeNode    // The pipeline in the action.
	HTMLContext *HTMLContext // Set by Template.Parse / AddParseTree; nil in non-HTML mode.
	count       atomic.Int64
	TrimLeft    bool // {{- was used
	TrimRight   bool // -}} was used
}

func (t *Tree) newAction(start, end lex.Pos, line int, pipe *PipeNode) *ActionNode {
	return &ActionNode{tr: t, start: start, end: end, Line: line, Pipe: pipe}
}

func (a *ActionNode) Start() lex.Pos { return a.start }
func (a *ActionNode) End() lex.Pos   { return a.end }
func (a *ActionNode) String() string { return a.tr.src(a.Start(), a.End()) }

func (a *ActionNode) tree() *Tree { return a.tr }

func (a *ActionNode) writeTo(sb *strings.Builder) {
	sb.WriteString("{{")
	a.Pipe.writeTo(sb)
	sb.WriteString("}}")
}

// CommandNode holds a command (a pipeline inside an evaluating action).
type CommandNode struct {
	tr         *Tree
	start, end lex.Pos
	Args       []Node // Arguments in lexical order: Identifier, field, or constant.
}

func (t *Tree) newCommand(start lex.Pos) CommandNode { return CommandNode{tr: t, start: start} }

func (c *CommandNode) Start() lex.Pos { return c.start }
func (c *CommandNode) End() lex.Pos   { return c.end }
func (c *CommandNode) String() string { return c.tr.src(c.Start(), c.End()) }
func (c *CommandNode) tree() *Tree    { return c.tr }

func (c *CommandNode) writeTo(sb *strings.Builder) {
	for i, arg := range c.Args {
		if i > 0 {
			sb.WriteByte(' ')
		}
		if arg, ok := arg.(*PipeNode); ok {
			sb.WriteByte('(')
			arg.writeTo(sb)
			sb.WriteByte(')')
			continue
		}
		arg.writeTo(sb)
	}
}

// IdentifierNode holds an identifier.
type IdentifierNode struct {
	tr         *Tree
	start, end lex.Pos
	Ident      string // The identifier's name.
}

// NewIdentifier returns a new [IdentifierNode] with the given identifier name.
func NewIdentifier(tr *Tree, start, end lex.Pos, ident string) *IdentifierNode {
	return &IdentifierNode{tr: tr, Ident: ident, start: start, end: end}
}

func (i *IdentifierNode) Start() lex.Pos { return i.start }
func (i *IdentifierNode) End() lex.Pos   { return i.end }
func (i *IdentifierNode) String() string { return i.tr.src(i.Start(), i.End()) }

func (i *IdentifierNode) writeTo(sb *strings.Builder) { sb.WriteString(i.String()) }
func (i *IdentifierNode) tree() *Tree                 { return i.tr }

// VariableNode holds a list of variable names, possibly with chained field
// accesses. The dollar sign is part of the (first) name.
type VariableNode struct {
	tr         *Tree
	start, end lex.Pos
	Ident      []string // Variable name and fields in lexical order.
}

func (t *Tree) newVariable(start, end lex.Pos, ident string) *VariableNode {
	return &VariableNode{tr: t, start: start, end: end, Ident: strings.Split(ident, ".")}
}

func (v *VariableNode) Start() lex.Pos { return v.start }
func (v *VariableNode) End() lex.Pos   { return v.end }
func (v *VariableNode) String() string { return v.tr.src(v.Start(), v.End()) }

func (v *VariableNode) tree() *Tree { return v.tr }

func (v *VariableNode) writeTo(sb *strings.Builder) {
	for i, id := range v.Ident {
		if i > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(id)
	}
}

// DotNode holds the special identifier '.'.
type DotNode struct {
	tr         *Tree
	start, end lex.Pos
}

func (t *Tree) newDot(start, end lex.Pos) *DotNode {
	return &DotNode{tr: t, start: start, end: end}
}

func (d *DotNode) Start() lex.Pos { return d.start }
func (d *DotNode) End() lex.Pos   { return d.end }
func (d *DotNode) String() string { return d.tr.src(d.Start(), d.End()) }

func (d *DotNode) writeTo(sb *strings.Builder) { sb.WriteString(d.String()) }
func (d *DotNode) tree() *Tree                 { return d.tr }

// NilNode holds the special identifier 'nil' representing an untyped nil constant.
type NilNode struct {
	tr         *Tree
	start, end lex.Pos
}

func (t *Tree) newNil(start, end lex.Pos) *NilNode {
	return &NilNode{tr: t, start: start, end: end}
}

func (n *NilNode) Start() lex.Pos { return n.start }
func (n *NilNode) End() lex.Pos   { return n.end }
func (n *NilNode) String() string { return n.tr.src(n.Start(), n.End()) }

func (n *NilNode) writeTo(sb *strings.Builder) { sb.WriteString(n.String()) }
func (n *NilNode) tree() *Tree                 { return n.tr }

// FieldNode holds a field (identifier starting with '.').
// The names may be chained ('.x.y').
// The period is dropped from each ident.
type FieldNode struct {
	tr         *Tree
	start, end lex.Pos
	Ident      []string // The identifiers in lexical order.
}

func (t *Tree) newField(pos, end lex.Pos, ident string) *FieldNode {
	return &FieldNode{tr: t, start: pos, end: end, Ident: strings.Split(ident[1:], ".")} // [1:] to drop leading period
}

func (f *FieldNode) Start() lex.Pos { return f.start }
func (f *FieldNode) End() lex.Pos   { return f.end }
func (f *FieldNode) String() string { return f.tr.src(f.Start(), f.End()) }

func (f *FieldNode) tree() *Tree { return f.tr }

func (f *FieldNode) writeTo(sb *strings.Builder) {
	for _, id := range f.Ident {
		sb.WriteByte('.')
		sb.WriteString(id)
	}
}

// ChainNode holds a term followed by a chain of field accesses (identifier starting with '.').
// The names may be chained ('.x.y').
// The periods are dropped from each ident.
type ChainNode struct {
	tr         *Tree
	start, end lex.Pos
	Node       Node
	Field      []string // The identifiers in lexical order.
}

func (t *Tree) newChain(start lex.Pos, node Node) *ChainNode {
	return &ChainNode{tr: t, start: start, Node: node}
}

func (c *ChainNode) Start() lex.Pos { return c.start }
func (c *ChainNode) End() lex.Pos   { return c.end }
func (c *ChainNode) String() string { return c.tr.src(c.Start(), c.End()) }

func (c *ChainNode) tree() *Tree { return c.tr }

// Add adds the named field (which should start with a period) to the end of the chain.
func (c *ChainNode) Add(field string) {
	if len(field) == 0 || field[0] != '.' {
		panic("no dot in field")
	}
	field = field[1:] // Remove leading dot.
	if field == "" {
		panic("empty field")
	}
	c.Field = append(c.Field, field)
}

func (c *ChainNode) writeTo(sb *strings.Builder) {
	if _, ok := c.Node.(*PipeNode); ok {
		sb.WriteByte('(')
		c.Node.writeTo(sb)
		sb.WriteByte(')')
	} else {
		c.Node.writeTo(sb)
	}
	for _, field := range c.Field {
		sb.WriteByte('.')
		sb.WriteString(field)
	}
}

// BoolNode holds a boolean constant.
type BoolNode struct {
	tr         *Tree
	start, end lex.Pos
	True       bool // The value of the boolean constant.
}

func (t *Tree) newBool(start, end lex.Pos, true bool) *BoolNode {
	return &BoolNode{tr: t, start: start, end: end, True: true}
}

func (b *BoolNode) Start() lex.Pos { return b.start }
func (b *BoolNode) End() lex.Pos   { return b.end }
func (b *BoolNode) String() string { return b.tr.src(b.Start(), b.End()) }

func (b *BoolNode) writeTo(sb *strings.Builder) { sb.WriteString(b.String()) }
func (b *BoolNode) tree() *Tree                 { return b.tr }

// StringNode holds a string constant. The value has been "unquoted".
type StringNode struct {
	tr         *Tree
	start, end lex.Pos
	Quoted     string // The original text of the string, with quotes.
	Text       string // The string, after quote processing.
}

func (t *Tree) newString(start, end lex.Pos, orig, text string) *StringNode {
	return &StringNode{tr: t, start: start, end: end, Quoted: orig, Text: text}
}

func (s *StringNode) Start() lex.Pos { return s.start }
func (s *StringNode) End() lex.Pos   { return s.end }
func (s *StringNode) String() string { return s.tr.src(s.Start(), s.End()) }

func (s *StringNode) writeTo(sb *strings.Builder) { sb.WriteString(s.String()) }
func (s *StringNode) tree() *Tree                 { return s.tr }

// endNode represents an {{end}} action.
// It does not appear in the final parse tree.
type endNode struct {
	tr         *Tree
	start, end lex.Pos
	TrimLeft   bool // {{- was used
	TrimRight  bool // -}} was used
}

func (t *Tree) newEnd(start, end lex.Pos) *endNode {
	return &endNode{tr: t, start: start, end: end}
}

func (e *endNode) Start() lex.Pos { return e.start }
func (e *endNode) End() lex.Pos   { return e.end }
func (e *endNode) String() string {
	return e.tr.src(e.Start(), e.End())
}

func (e *endNode) writeTo(sb *strings.Builder) { sb.WriteString(e.String()) }
func (e *endNode) tree() *Tree                 { return e.tr }

// elseNode represents an {{else}} action. Does not appear in the final tree.
type elseNode struct {
	tr         *Tree
	start, end lex.Pos
	Line       int  // The line number in the input. Deprecated: Kept for compatibility.
	TrimLeft   bool // {{- was used
	TrimRight  bool // -}} was used
}

func (t *Tree) newElse(start, end lex.Pos, line int) *elseNode {
	return &elseNode{tr: t, start: start, end: end, Line: line}
}

func (e *elseNode) Start() lex.Pos { return e.start }
func (e *elseNode) End() lex.Pos   { return e.end }
func (e *elseNode) String() string { return e.tr.src(e.start, e.End()) }

func (e *elseNode) writeTo(sb *strings.Builder) { sb.WriteString(e.String()) }
func (e *elseNode) tree() *Tree                 { return e.tr }

// BranchNode is the common representation of if, range, and with.
type BranchNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Line       int          // The line number in the input. Deprecated: Kept for compatibility.
	Pipe       *PipeNode    // The pipeline to be evaluated.
	List       *ListNode    // What to execute if the value is non-empty.
	ElseList   *ListNode    // What to execute if the value is empty (nil if absent).
	openCount  atomic.Int64 // hit when condition evaluated
	elseCount  atomic.Int64 // hit when else entered
	endCount   atomic.Int64 // hit on normal completion only

	StartTrimLeft  bool // {{- on opener
	StartTrimRight bool // -}} on opener
	ElseTrimLeft   bool // {{- on else
	ElseTrimRight  bool // -}} on else
	EndTrimLeft    bool // {{- on end
	EndTrimRight   bool // -}} on end
}

func (b *BranchNode) Start() lex.Pos { return b.start }
func (b *BranchNode) End() lex.Pos   { return b.end }
func (b *BranchNode) String() string {
	return b.tr.src(b.Start(), b.End())
}

func (b *BranchNode) tree() *Tree { return b.tr }

func (b *BranchNode) writeTo(sb *strings.Builder, keyword string) {
	sb.WriteString("{{")
	sb.WriteString(keyword)
	sb.WriteByte(' ')
	b.Pipe.writeTo(sb)
	sb.WriteString("}}")
	b.List.writeTo(sb)
	if b.ElseList != nil {
		sb.WriteString("{{else}}")
		b.ElseList.writeTo(sb)
	}
	sb.WriteString("{{end}}")
}

// IfNode represents an {{if}} action and its commands.
type IfNode struct {
	BranchNode
}

func (t *Tree) newIf(pos, end lex.Pos, line int, pipe *PipeNode, list, elseList *ListNode) *IfNode {
	return &IfNode{BranchNode: BranchNode{tr: t, start: pos, end: end, Line: line, Pipe: pipe, List: list, ElseList: elseList}}
}

func (i *IfNode) writeTo(builder *strings.Builder) { i.BranchNode.writeTo(builder, "if") }

// BreakNode represents a {{break}} action.
type BreakNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Line       int
	count      atomic.Int64
	TrimLeft   bool // {{- was used
	TrimRight  bool // -}} was used
}

func (t *Tree) newBreak(pos, end lex.Pos, line int) *BreakNode {
	return &BreakNode{tr: t, start: pos, end: end, Line: line}
}

func (b *BreakNode) Start() lex.Pos              { return b.start }
func (b *BreakNode) End() lex.Pos                { return b.end }
func (b *BreakNode) String() string              { return b.tr.src(b.Start(), b.End()) }
func (b *BreakNode) tree() *Tree                 { return b.tr }
func (b *BreakNode) writeTo(sb *strings.Builder) { sb.WriteString("{{break}}") }

// ContinueNode represents a {{continue}} action.
type ContinueNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Line       int
	count      atomic.Int64
	TrimLeft   bool // {{- was used
	TrimRight  bool // -}} was used
}

func (t *Tree) newContinue(start, end lex.Pos, line int) *ContinueNode {
	return &ContinueNode{tr: t, start: start, end: end, Line: line}
}

func (c *ContinueNode) Start() lex.Pos              { return c.start }
func (c *ContinueNode) End() lex.Pos                { return c.end }
func (c *ContinueNode) String() string              { return c.tr.src(c.Start(), c.End()) }
func (c *ContinueNode) tree() *Tree                 { return c.tr }
func (c *ContinueNode) writeTo(sb *strings.Builder) { sb.WriteString("{{continue}}") }

// RangeNode represents a {{range}} action and its commands.
type RangeNode struct {
	BranchNode
}

func (t *Tree) newRange(start, end lex.Pos, line int, pipe *PipeNode, list, elseList *ListNode) *RangeNode {
	return &RangeNode{BranchNode: BranchNode{tr: t, start: start, end: end, Line: line, Pipe: pipe, List: list, ElseList: elseList}}
}

func (r *RangeNode) writeTo(builder *strings.Builder) { r.BranchNode.writeTo(builder, "range") }

// WithNode represents a {{with}} action and its commands.
type WithNode struct {
	BranchNode
}

func (t *Tree) newWith(start, end lex.Pos, line int, pipe *PipeNode, list, elseList *ListNode) *WithNode {
	return &WithNode{BranchNode: BranchNode{tr: t, start: start, end: end, Line: line, Pipe: pipe, List: list, ElseList: elseList}}
}

func (t *WithNode) writeTo(builder *strings.Builder) { t.BranchNode.writeTo(builder, "with") }

// TemplateNode represents a {{template}} action.
type TemplateNode struct {
	nodeParent
	tr          *Tree
	start, end  lex.Pos
	Line        int          // The line number in the input. Deprecated: Kept for compatibility.
	Name        string       // The name of the template (unquoted).
	Pipe        *PipeNode    // The command to evaluate as dot for the template.
	HTMLContext *HTMLContext // Set by AnnotateHTMLContexts; nil in non-HTML mode.
	count       atomic.Int64
	TrimLeft    bool // {{- was used
	TrimRight   bool // -}} was used
}

func (t *Tree) newTemplate(start, end lex.Pos, line int, name string, pipe *PipeNode) *TemplateNode {
	return &TemplateNode{tr: t, start: start, end: end, Line: line, Name: name, Pipe: pipe}
}

func (t *TemplateNode) Start() lex.Pos { return t.start }
func (t *TemplateNode) End() lex.Pos   { return t.end }
func (t *TemplateNode) String() string {
	return t.tr.src(t.Start(), t.End())
}

func (t *TemplateNode) tree() *Tree { return t.tr }

func (t *TemplateNode) writeTo(sb *strings.Builder) {
	sb.WriteString("{{template ")
	sb.WriteString(strconv.Quote(t.Name))
	if t.Pipe != nil {
		sb.WriteByte(' ')
		t.Pipe.writeTo(sb)
	}
	sb.WriteString("}}")
}

// DefineNode represents a {{define "name"}}...{{end}} kept inline in the AST.
// In standard text/template parsing, defines are extracted into separate trees
// during parsing. DefineNode allows the parser to retain the define in the
// source-faithful AST; Extract() can later pull them out for execution.
type DefineNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Line       int       // The line number in the input.
	Name       string    // The name of the defined template (unquoted).
	Body       *ListNode // The body of the define.

	StartTrimLeft  bool // {{- on define opener
	StartTrimRight bool // -}} on define opener
	EndTrimLeft    bool // {{- on end
	EndTrimRight   bool // -}} on end
}

func (t *Tree) newDefine(start, end lex.Pos, line int, name string, body *ListNode) *DefineNode {
	return &DefineNode{tr: t, start: start, end: end, Line: line, Name: name, Body: body}
}

func (d *DefineNode) Start() lex.Pos { return d.start }
func (d *DefineNode) End() lex.Pos   { return d.end }
func (d *DefineNode) String() string {
	return d.tr.src(d.Start(), d.End())
}
func (d *DefineNode) tree() *Tree { return d.tr }

func (d *DefineNode) writeTo(sb *strings.Builder) {
	sb.WriteString("{{define ")
	sb.WriteString(strconv.Quote(d.Name))
	sb.WriteString("}}")
	d.Body.writeTo(sb)
	sb.WriteString("{{end}}")
}

// BlockNode represents a {{block "name" pipeline}}...{{end}} kept inline in
// the AST. Like DefineNode it retains the body in the source tree; Extract()
// pulls it out for execution. Unlike DefineNode it carries a Pipe (like
// TemplateNode) that evaluates to dot for the block body.
type BlockNode struct {
	nodeParent
	tr         *Tree
	start, end lex.Pos
	Line       int       // The line number in the input.
	Name       string    // The name of the block template (unquoted).
	Pipe       *PipeNode // The pipeline to evaluate as dot for the block.
	Body       *ListNode // The body of the block.

	StartTrimLeft  bool // {{- on block opener
	StartTrimRight bool // -}} on block opener
	EndTrimLeft    bool // {{- on end
	EndTrimRight   bool // -}} on end

	call *TemplateNode
}

func (t *Tree) newBlock(start, end lex.Pos, line int, name string, pipe *PipeNode, body *ListNode) *BlockNode {
	return &BlockNode{
		tr: t, start: start, end: end, Line: line, Name: name, Pipe: pipe, Body: body,
		call: &TemplateNode{tr: t, start: start, end: end, Line: line, Name: name, Pipe: pipe},
	}
}

// Call returns the {{template}} invocation text/template makes of a
// block: a call of the template named Name, with Pipe as its dot, written
// where the block is. The body is that template's default definition,
// which a later define of the same name replaces, so a checker or walker
// following the invocation reaches whichever definition the set kept.
// It spans the whole block clause and is the same node on every call.
func (b *BlockNode) Call() *TemplateNode { return b.call }

func (b *BlockNode) Start() lex.Pos { return b.start }
func (b *BlockNode) End() lex.Pos   { return b.end }
func (b *BlockNode) String() string {
	return b.tr.src(b.Start(), b.End())
}
func (b *BlockNode) tree() *Tree { return b.tr }

func (b *BlockNode) writeTo(sb *strings.Builder) {
	sb.WriteString("{{block ")
	sb.WriteString(strconv.Quote(b.Name))
	sb.WriteByte(' ')
	b.Pipe.writeTo(sb)
	sb.WriteString("}}")
	b.Body.writeTo(sb)
	sb.WriteString("{{end}}")
}
