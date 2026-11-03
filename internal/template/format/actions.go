package format

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// errActionChanged is the error Source returns when its output does not
// parse to the same actions as its input: an action printer bug.
var errActionChanged = errors.New("format: printed actions do not parse as written")

// errActionUnread is the error Source returns when the emitter cannot
// find an action of the tree in the source to print it: a formatter
// bug, reported rather than leaving the action unformatted.
var errActionUnread = errors.New("format: an action could not be read for printing")

// actionShapes describes, in source order, every action-bearing node of
// root as its parse tree has it, positions and spacing aside: kind,
// trim markers, names, declarations, commands and their arguments, with
// literals as written. The parentheses printPipe drops (redundantParens)
// are dropped here too, so a source and its formatted output have the
// same shapes. Each name is written as rename returns it, when rename is
// not nil. It is independent of the printer: it reads the tree only.
func actionShapes(root *template.ListNode, rename func(string) string) []string {
	if rename == nil {
		rename = func(name string) string { return name }
	}
	var shapes []string
	template.WalkPath(&template.Tree{Root: root}, func(_ *template.Tree, path []template.Node, n template.Node) {
		var b strings.Builder
		b.WriteString(shapeNesting(path))
		switch n := n.(type) {
		case *template.ActionNode:
			b.WriteString("action")
			shapeTrims(&b, n.TrimLeft, n.TrimRight)
			shapePipe(&b, n.Pipe)
		case *template.IfNode:
			shapeBranch(&b, "if", &n.BranchNode)
		case *template.WithNode:
			shapeBranch(&b, "with", &n.BranchNode)
		case *template.RangeNode:
			shapeBranch(&b, "range", &n.BranchNode)
		case *template.TemplateNode:
			b.WriteString("template " + strconv.Quote(rename(n.Name)))
			shapeTrims(&b, n.TrimLeft, n.TrimRight)
			if n.Pipe != nil {
				shapePipe(&b, n.Pipe)
			}
		case *template.BlockNode:
			b.WriteString("block " + strconv.Quote(rename(n.Name)))
			shapeTrims(&b, n.StartTrimLeft, n.StartTrimRight, n.EndTrimLeft, n.EndTrimRight)
			shapePipe(&b, n.Pipe)
		case *template.DefineNode:
			b.WriteString("define " + strconv.Quote(rename(n.Name)))
			shapeTrims(&b, n.StartTrimLeft, n.StartTrimRight, n.EndTrimLeft, n.EndTrimRight)
		case *template.BreakNode:
			b.WriteString("break")
			shapeTrims(&b, n.TrimLeft, n.TrimRight)
		case *template.ContinueNode:
			b.WriteString("continue")
			shapeTrims(&b, n.TrimLeft, n.TrimRight)
		case *template.CommentNode:
			// A comment's continuation lines may shift (shiftComment).
			b.WriteString("comment " + strconv.Quote(strings.Join(strings.Fields(n.Text), " ")))
			shapeTrims(&b, n.TrimLeft, n.TrimRight)
		default:
			return
		}
		shapes = append(shapes, b.String())
	})
	return shapes
}

// shapeNesting describes where a node with ancestors path sits among
// the template's structures: each enclosing branch, define and block,
// and for a branch whether the node is in its body or its else list.
func shapeNesting(path []template.Node) string {
	var b strings.Builder
	for i, n := range path {
		var kind string
		var branch *template.BranchNode
		switch n := n.(type) {
		case *template.IfNode:
			kind, branch = "if", &n.BranchNode
		case *template.WithNode:
			kind, branch = "with", &n.BranchNode
		case *template.RangeNode:
			kind, branch = "range", &n.BranchNode
		case *template.DefineNode:
			kind = "define"
		case *template.BlockNode:
			kind = "block"
		default:
			continue
		}
		b.WriteString(kind)
		if branch != nil && i+1 < len(path) && branch.ElseList != nil && path[i+1] == template.Node(branch.ElseList) {
			b.WriteString(".else")
		}
		b.WriteString("/")
	}
	return b.String()
}

// definedNames returns the distinct names root's defines and blocks
// declare.
func definedNames(root *template.ListNode) map[string]bool {
	names := make(map[string]bool)
	template.Walk(&template.Tree{Root: root}, func(_ *template.Tree, n template.Node) {
		switch n := n.(type) {
		case *template.DefineNode:
			names[n.Name] = true
		case *template.BlockNode:
			names[n.Name] = true
		}
	})
	return names
}

func shapeBranch(b *strings.Builder, keyword string, n *template.BranchNode) {
	b.WriteString(keyword)
	switch {
	case n.ElseChain() != nil:
		b.WriteString(" chained")
	case n.ElseList != nil:
		b.WriteString(" else")
	}
	shapeTrims(b, n.StartTrimLeft, n.StartTrimRight, n.ElseTrimLeft, n.ElseTrimRight, n.EndTrimLeft, n.EndTrimRight)
	shapePipe(b, n.Pipe)
}

func shapeTrims(b *strings.Builder, trims ...bool) {
	b.WriteString(" [")
	for _, trim := range trims {
		if trim {
			b.WriteByte('-')
		} else {
			b.WriteByte('.')
		}
	}
	b.WriteString("]")
}

func shapePipe(b *strings.Builder, p *template.PipeNode) {
	b.WriteString(" pipe(")
	for _, v := range p.Decl {
		b.WriteString("decl " + strings.Join(v.Ident, ".") + " ")
	}
	if len(p.Decl) > 0 && p.IsAssign {
		b.WriteString("assign ")
	}
	for i, c := range p.Commands {
		b.WriteString(" cmd(")
		for j, arg := range c.Args {
			if j == 0 && i == 0 && len(c.Args) == 1 {
				arg = redundantParens(arg)
			}
			shapeArg(b, arg)
		}
		b.WriteString(")")
	}
	b.WriteString(")")
}

func shapeArg(b *strings.Builder, n template.Node) {
	switch n := n.(type) {
	case *template.PipeNode:
		shapePipe(b, n)
	case *template.ChainNode:
		b.WriteString(" chain(")
		shapeArg(b, n.Node)
		b.WriteString(" ." + strings.Join(n.Field, ".") + ")")
	case *template.FieldNode:
		b.WriteString(" field ." + strings.Join(n.Ident, "."))
	case *template.VariableNode:
		b.WriteString(" var " + strings.Join(n.Ident, "."))
	case *template.IdentifierNode:
		b.WriteString(" ident " + n.Ident)
	case *template.DotNode:
		b.WriteString(" dot")
	case *template.NilNode:
		b.WriteString(" nil")
	case *template.BoolNode:
		b.WriteString(" bool " + strconv.FormatBool(n.True))
	case *template.NumberNode:
		b.WriteString(" number " + n.Text)
	case *template.StringNode:
		b.WriteString(" string " + n.Quoted)
	default:
		b.WriteString(" unknown")
	}
}

// redundantParens returns the operand n stands for when n is a
// parenthesized pipeline whose parentheses change nothing at the head
// of the first command of a pipeline that has no other argument: a
// pipeline without declarations holding one command of one argument
// that is a field, variable, dot, constant or nil (itself perhaps
// parenthesized so). text/template evaluates such a command the same
// with or without them. A call (an identifier) keeps them, as does any
// other place: as an argument, (.X) unwraps an any-typed nil that .X
// passes as is, and a constant takes the parameter's type only bare; in
// a later stage, parentheses refuse the piped value that a bare field
// or variable takes as a method argument.
func redundantParens(n template.Node) template.Node {
	p, ok := n.(*template.PipeNode)
	if !ok || len(p.Decl) > 0 || len(p.Commands) != 1 || len(p.Commands[0].Args) != 1 {
		return n
	}
	switch inner := redundantParens(p.Commands[0].Args[0]); inner.(type) {
	case *template.FieldNode, *template.VariableNode, *template.DotNode, *template.NilNode,
		*template.BoolNode, *template.NumberNode, *template.StringNode:
		return inner
	}
	return n
}

// actionToken is one action-bearing token of the source: an action, a
// branch, define or block opener, an {{else}} or {{end}}, a {{template}},
// {{break}} or {{continue}}. The formatter prints it from its parse tree
// (printToken) unless it is left as written (asWritten).
type actionToken struct {
	start, end          int // offsets in the emitter's source, delimiters included
	trimLeft, trimRight bool
	// head is what precedes the pipeline: the keyword and, for
	// template, block and define, the name literal as written ("" for a
	// plain action).
	head string
	// named reports a template, block or define token: name is the
	// name it parses to and literal the name literal as written.
	named         bool
	name, literal string
	pipe          *template.PipeNode // nil when the token has none
	// asWritten reports that the token is copied as it is: it is in a
	// muxt:fmt region, or a string literal in it spans lines.
	asWritten bool
}

// tokenSpan is what scanAction reads of a token.
type tokenSpan struct {
	end                 int
	trimLeft, trimRight bool
	inner               string // between the delimiters and trim markers
	literalBreak        bool   // a raw string literal in it holds a line break
}

// scanAction reads the action token whose left delimiter is at src[at]:
// its end, trim markers, interior, and whether a raw string literal in
// it spans lines. It reports false when src holds no complete token
// there. The right delimiter is the first outside a literal, as the
// lexer reads it.
func scanAction(src string, at int, left, right string) (tokenSpan, bool) {
	var s tokenSpan
	if at < 0 || at > len(src) || !strings.HasPrefix(src[at:], left) {
		return s, false
	}
	k := at + len(left)
	if hasLeftTrim(src[k:]) {
		s.trimLeft = true
		k += 2
	}
	from := k
	for k < len(src) {
		switch c := src[k]; c {
		case '"', '\'':
			j := k + 1
			for j < len(src) && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(src) || src[j] != c {
				return s, false
			}
			k = j + 1
			continue
		case '`':
			j := strings.IndexByte(src[k+1:], '`')
			if j < 0 {
				return s, false
			}
			if strings.Contains(src[k+1:k+1+j], "\n") {
				s.literalBreak = true
			}
			k += j + 2
			continue
		}
		if hasRightTrim(src[k:]) && strings.HasPrefix(src[k+2:], right) {
			s.trimRight = true
			s.inner, s.end = src[from:k], k+2+len(right)
			return s, true
		}
		if strings.HasPrefix(src[k:], right) {
			s.inner, s.end = src[from:k], k+len(right)
			return s, true
		}
		k++
	}
	return s, false
}

func isActionSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func hasLeftTrim(s string) bool { return len(s) >= 2 && s[0] == '-' && isActionSpace(s[1]) }

func hasRightTrim(s string) bool { return len(s) >= 2 && isActionSpace(s[0]) && s[1] == '-' }

// nameLiteral returns the first string literal of inner, the interior of
// a template, block or define token, as written.
func nameLiteral(inner string) (string, bool) {
	i := strings.IndexAny(inner, "\"`")
	if i < 0 {
		return "", false
	}
	if inner[i] == '`' {
		j := strings.IndexByte(inner[i+1:], '`')
		if j < 0 {
			return "", false
		}
		return inner[i : i+j+2], true
	}
	for j := i + 1; j < len(inner); j++ {
		switch inner[j] {
		case '\\':
			j++
		case '"':
			return inner[i : j+1], true
		}
	}
	return "", false
}

// collectActions finds the action-bearing tokens of root, in source
// order, for the emitter to print.
func (e *emitter) collectActions(root *template.ListNode, left, right string) {
	chained := make(map[*template.BranchNode]bool)
	add := func(start lex.Pos, wantEnd lex.Pos, keyword string, named bool, name string, pipe *template.PipeNode) {
		tok, ok := e.scanToken(start, wantEnd, keyword, named, left, right)
		if !ok {
			e.unread = true
			if named {
				e.unprinted = append(e.unprinted, name)
			}
			return
		}
		tok.name, tok.pipe = name, pipe
		e.actions = append(e.actions, tok)
	}
	template.Walk(&template.Tree{Root: root}, func(_ *template.Tree, n template.Node) {
		switch n := n.(type) {
		case *template.ActionNode:
			add(n.Start(), n.End(), "", false, "", n.Pipe)
		case *template.TemplateNode:
			add(n.Start(), n.End(), "template", true, n.Name, n.Pipe)
		case *template.BreakNode:
			add(n.Start(), n.End(), "break", false, "", nil)
		case *template.ContinueNode:
			add(n.Start(), n.End(), "continue", false, "", nil)
		case *template.IfNode:
			e.collectBranch(add, "if", &n.BranchNode, chained)
		case *template.WithNode:
			e.collectBranch(add, "with", &n.BranchNode, chained)
		case *template.RangeNode:
			e.collectBranch(add, "range", &n.BranchNode, chained)
		case *template.DefineNode:
			if n.Body == nil {
				e.unread = true
				e.unprinted = append(e.unprinted, n.Name)
				return
			}
			add(n.Start(), -1, "define", true, n.Name, nil)
			add(n.Body.End(), n.End(), "end", false, "", nil)
		case *template.BlockNode:
			if n.Body == nil || n.Pipe == nil {
				e.unread = true
				e.unprinted = append(e.unprinted, n.Name)
				return
			}
			add(n.Start(), n.Pipe.End(), "block", true, n.Name, n.Pipe)
			add(n.Body.End(), n.End(), "end", false, "", nil)
		}
	})
	slices.SortFunc(e.actions, func(a, b *actionToken) int { return a.start - b.start })
	// Tokens never overlap; should two, leave both as written and
	// report it.
	for i := 1; i < len(e.actions); i++ {
		if e.actions[i].start < e.actions[i-1].end {
			e.actions[i].asWritten, e.actions[i-1].asWritten = true, true
			e.unread = true
		}
	}
}

// hasKeyword reports whether inner, a token's interior, starts with the
// words of keyword ("if", "else if"), each after optional whitespace and
// ending where an identifier would: {{if(.X)}} holds "if".
func hasKeyword(inner, keyword string) bool {
	for _, word := range strings.Fields(keyword) {
		inner = strings.TrimLeft(inner, " \t\r\n")
		if !strings.HasPrefix(inner, word) {
			return false
		}
		inner = inner[len(word):]
		if inner != "" && isIdentByte(inner[0]) {
			return false
		}
	}
	return true
}

func isIdentByte(c byte) bool {
	return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c >= 0x80
}

// scanToken reads the token for keyword whose left delimiter is at
// start and, unless wantEnd is -1, whose end is at wantEnd. It reports
// false when the source there is not that token.
func (e *emitter) scanToken(start, wantEnd lex.Pos, keyword string, named bool, left, right string) (*actionToken, bool) {
	at := int(start - e.base)
	s, ok := scanAction(e.src, at, left, right)
	if !ok || wantEnd >= 0 && s.end != int(wantEnd-e.base) {
		return nil, false
	}
	if !hasKeyword(s.inner, keyword) {
		return nil, false
	}
	tok := &actionToken{
		start: at, end: s.end,
		trimLeft: s.trimLeft, trimRight: s.trimRight,
		head:      keyword,
		named:     named,
		asWritten: s.literalBreak || e.inVerbatim(at),
	}
	if named {
		literal, ok := nameLiteral(s.inner)
		if !ok {
			return nil, false
		}
		tok.literal = literal
		tok.head += " " + literal
	}
	return tok, true
}

// collectBranch adds b's opener ({{else if}} or {{else with}} when b is
// chained onto another branch's else), its plain {{else}}, and its
// {{end}}, which a chain shares with its last branch.
func (e *emitter) collectBranch(add func(start, wantEnd lex.Pos, keyword string, named bool, name string, pipe *template.PipeNode), keyword string, b *template.BranchNode, chained map[*template.BranchNode]bool) {
	if b.List == nil || b.Pipe == nil {
		e.unread = true
		return
	}
	opener := keyword
	if chained[b] {
		opener = "else " + keyword
	}
	add(b.Start(), b.Pipe.End(), opener, false, "", b.Pipe)
	chain := b.ElseChain()
	if chain != nil {
		chained[chain] = true
		return
	}
	last := b.List
	if b.ElseList != nil {
		add(b.List.End(), -1, "else", false, "", nil)
		last = b.ElseList
	}
	add(last.End(), b.End(), "end", false, "", nil)
}

// printToken returns tok printed from its parse tree: its delimiters
// and trim markers, one space between words, " | " between pipeline
// stages, literals and names as written, and the parentheses the
// grammar needs (redundantParens). A pipeline the source breaks between
// stages and a command the source breaks between arguments are laid out
// one stage or argument per line, each continuation line one tab deeper
// than the line it continues (indent is the indentation of the line the
// token starts on); the closing delimiter then goes on a line of its own
// at indent. When the token starts its line (lineStart: only whitespace
// before it), so is a pipeline with four or more stages and a command
// with three or more parenthesized arguments; in a start tag or inside
// text such a token stays on one line.
func (e *emitter) printToken(tok *actionToken, left, right, indent string, lineStart bool) string {
	p := actionPrinter{src: e.src, base: e.base, indent: indent, auto: lineStart}
	p.b.WriteString(left)
	if tok.trimLeft {
		p.b.WriteString("- ")
	}
	p.b.WriteString(tok.head)
	if tok.pipe != nil && tok.head != "" {
		p.b.WriteByte(' ')
	}
	if tok.pipe != nil {
		p.pipe(tok.pipe, 0)
	}
	if strings.Contains(p.b.String(), "\n") {
		p.newline(0)
		if tok.trimRight {
			p.b.WriteByte('-')
		}
	} else if tok.trimRight {
		p.b.WriteString(" -")
	}
	p.b.WriteString(right)
	return p.b.String()
}

type actionPrinter struct {
	src    string
	base   lex.Pos
	indent string
	auto   bool // split complex pipelines and commands (printToken)
	b      strings.Builder
}

func (p *actionPrinter) newline(level int) {
	p.b.WriteByte('\n')
	p.b.WriteString(p.indent)
	p.b.WriteString(strings.Repeat("\t", level))
}

// between returns the source from the end of a to the start of b.
func (p *actionPrinter) between(a, b template.Node) string {
	from, to := int(a.End()-p.base), int(b.Start()-p.base)
	if from < 0 || to > len(p.src) || from > to {
		return ""
	}
	return p.src[from:to]
}

// pipe prints pl, whose first line is at level: its declarations, then
// its commands, one per line one level deeper when it splits. It
// reports whether pl split: its stages or the arguments of one of its
// commands.
func (p *actionPrinter) pipe(pl *template.PipeNode, level int) bool {
	for i, v := range pl.Decl {
		if i > 0 {
			p.b.WriteString(", ")
		}
		p.b.WriteString(strings.Join(v.Ident, "."))
	}
	if len(pl.Decl) > 0 {
		if pl.IsAssign {
			p.b.WriteString(" = ")
		} else {
			p.b.WriteString(" := ")
		}
	}
	split, splitArgs := p.stagesSplit(pl), false
	for i, c := range pl.Commands {
		switch {
		case i == 0 && split:
			// The first command's arguments nest under the stage
			// lines, so a later "|" never reads as an argument.
			splitArgs = p.command(c, level+1, true)
		case i == 0:
			splitArgs = p.command(c, level, true)
		case split:
			p.newline(level + 1)
			p.b.WriteString("| ")
			splitArgs = p.command(c, level+1, false) || splitArgs
		default:
			p.b.WriteString(" | ")
			splitArgs = p.command(c, level, false) || splitArgs
		}
	}
	return split || splitArgs
}

// stagesSplit reports whether pl's stages go one per line: it has four
// or more (when splitting is automatic), the source breaks between two
// of them, or it has two or more and one command's arguments split.
func (p *actionPrinter) stagesSplit(pl *template.PipeNode) bool {
	if p.auto && len(pl.Commands) >= 4 {
		return true
	}
	for i := 1; i < len(pl.Commands); i++ {
		if strings.Contains(p.between(pl.Commands[i-1], pl.Commands[i]), "\n") {
			return true
		}
	}
	if len(pl.Commands) < 2 {
		return false
	}
	return slices.ContainsFunc(pl.Commands, p.argsSplit)
}

// argsSplit reports whether c's arguments go one per line: it has three
// or more parenthesized ones (when splitting is automatic), or the
// source breaks between two of them.
func (p *actionPrinter) argsSplit(c *template.CommandNode) bool {
	parens := 0
	for _, arg := range c.Args {
		if parenthesized(arg) {
			parens++
		}
	}
	if p.auto && parens >= 3 {
		return true
	}
	for i := 1; i < len(c.Args); i++ {
		gap := p.between(c.Args[i-1], c.Args[i])
		if _, ok := c.Args[i].(*template.PipeNode); ok {
			// A pipeline argument starts after its "(": a break
			// between the two is inside the parentheses.
			if j := strings.LastIndexByte(gap, '('); j >= 0 {
				gap = gap[:j]
			}
		}
		if strings.Contains(gap, "\n") {
			return true
		}
	}
	return false
}

// command prints c, whose line is at level; first reports that c is
// the first command of its pipeline. It reports whether c's arguments
// split.
func (p *actionPrinter) command(c *template.CommandNode, level int, first bool) bool {
	split := p.argsSplit(c)
	for i, arg := range c.Args {
		switch {
		case i == 0:
			if first && len(c.Args) == 1 {
				arg = redundantParens(arg)
			}
			p.operand(arg, level)
		case split:
			p.newline(level + 1)
			p.operand(arg, level+1)
		default:
			p.b.WriteByte(' ')
			p.operand(arg, level)
		}
	}
	return split
}

func parenthesized(n template.Node) bool {
	switch n := n.(type) {
	case *template.PipeNode:
		return true
	case *template.ChainNode:
		_, ok := n.Node.(*template.PipeNode)
		return ok
	}
	return false
}

// operand prints n, whose line is at level. A parenthesized pipeline
// that splits closes on a line of its own at level.
func (p *actionPrinter) operand(n template.Node, level int) {
	switch n := n.(type) {
	case *template.PipeNode:
		p.b.WriteByte('(')
		if p.pipe(n, level) {
			p.newline(level)
		}
		p.b.WriteByte(')')
	case *template.ChainNode:
		p.operand(n.Node, level)
		for _, f := range n.Field {
			p.b.WriteString("." + f)
		}
	case *template.FieldNode:
		for _, id := range n.Ident {
			p.b.WriteString("." + id)
		}
	case *template.VariableNode:
		p.b.WriteString(strings.Join(n.Ident, "."))
	case *template.IdentifierNode:
		p.b.WriteString(n.Ident)
	case *template.DotNode:
		p.b.WriteByte('.')
	case *template.NilNode:
		p.b.WriteString("nil")
	case *template.BoolNode:
		p.b.WriteString(strconv.FormatBool(n.True))
	case *template.NumberNode:
		p.b.WriteString(n.Text)
	case *template.StringNode:
		p.b.WriteString(n.Quoted)
	default:
		p.b.WriteString(n.String())
	}
}

// lineIndent returns the spaces and tabs that start the last line of s,
// and whether that line holds nothing else.
func lineIndent(s string) (indent string, blank bool) {
	line := s[strings.LastIndexByte(s, '\n')+1:]
	rest := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(rest)], rest == ""
}
