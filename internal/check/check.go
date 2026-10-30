package check

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// Error wraps a check failure with its source location. The single-line
// format is
//
//	{file}:{line}:{col}: executing {tree-name} at <{node-text}>: {message}
//
// The leading file:line:col is recognized by terminals and IDEs as a
// jump-to-source location.
type Error struct {
	Tree *template.Tree
	Node template.Node
	err  error
}

func newError(tree *template.Tree, node template.Node, message string, args ...any) *Error {
	return &Error{
		Tree: tree,
		Node: node,
		err:  fmt.Errorf(message, args...),
	}
}

func wrapError(tree *template.Tree, node template.Node, err error) *Error {
	return &Error{
		Tree: tree,
		Node: node,
		err:  err,
	}
}

func (e *Error) Error() string {
	pos := e.Tree.Position(e.Node)
	loc := cmp.Or(e.Tree.Filepath, pos.ParseName)
	return fmt.Sprintf("%s:%d:%d: executing %q at <%s>: %s",
		loc, pos.Line, pos.Column, e.Tree.Name, pos.Source, e.err.Error())
}

func (e *Error) Unwrap() error {
	return e.err
}

// Position returns the start of the error's node as a token.Position: the
// template file (its ParseName when it has no file path), the byte Offset,
// and the 1-based Line/Column. Tooling uses it — together with EndPosition
// and the position-free message from Unwrap — to place a diagnostic without
// parsing the Error string's location prefix.
func (e *Error) Position() token.Position {
	return e.positionAt(e.Node.Start())
}

// EndPosition returns the exclusive end of the error's node, the companion
// to Position for a ranged diagnostic.
func (e *Error) EndPosition() token.Position {
	return e.positionAt(e.Node.End())
}

func (e *Error) positionAt(p lex.Pos) token.Position {
	pos := e.Tree.PositionAt(p)
	return token.Position{
		Filename: cmp.Or(e.Tree.Filepath, e.Tree.ParseName),
		Offset:   int(p),
		Line:     pos.Line,
		Column:   pos.Column,
	}
}

// VerboseError returns a (possibly) multi-line message. The single-line
// summary returned by Error stays on the first line; if the wrapped error
// implements VerboseErrorer and contributes additional detail, that detail
// is indented on subsequent lines. A blank line within that detail (used,
// for example, to separate the summary from a rendered type declaration)
// stays empty rather than becoming a whitespace-only "  " line.
func (e *Error) VerboseError() string {
	pos := e.Tree.Position(e.Node)
	loc := cmp.Or(e.Tree.Filepath, pos.ParseName)
	prefix := fmt.Sprintf("%s:%d:%d: executing %q at <%s>: ",
		loc, pos.Line, pos.Column, e.Tree.Name, pos.Source)
	var v VerboseErrorer
	if !errors.As(e.err, &v) {
		return prefix + e.err.Error()
	}
	verbose := v.VerboseError()
	lines := strings.Split(verbose, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = "  " + lines[i]
		}
	}
	return prefix + strings.Join(lines, "\n")
}

// DetailedError writes the verbose, position-prefixed rendering of e to w.
// The qualifier is accepted for signature compatibility with consumers that
// pass one; htt renders type names its own way and does not use it.
func (e *Error) DetailedError(w io.Writer, _ types.Qualifier) error {
	_, err := io.WriteString(w, e.VerboseError())
	return err
}

type Global struct {
	trees TreeFinder
	calls CallChecker

	pkg             *types.Package
	fileSet         *token.FileSet
	typeNodeMapping TypeNodeMapping

	InspectTemplateNode TemplateNodeInspectorFunc
	InspectCallNode     ExecuteTemplateNodeInspectorFunc

	// InspectNode, when set, is called for every template node the
	// checker resolves a type for, in the context of the current
	// execution (the data type Execute started from flows down through
	// nested {{template}} invocations). Tooling such as the LSP uses it
	// to build an offset->type index. It never affects checking.
	InspectNode NodeInspectorFunc

	// InspectWarning, when set, receives non-fatal warnings (likely bugs
	// that are not type errors) as the checker walks. It never affects
	// checking; a nil sink means warnings are not computed at all.
	InspectWarning func(Warning)

	// Qualifier controls how types are printed in error messages.
	// If nil, types are printed with their full package path.
	// See types.WriteType for details.
	Qualifier types.Qualifier
}

type TemplateNodeInspectorFunc func(node *template.TemplateNode, t *template.Tree, tp types.Type)

// NodeTypeInfo describes a template node's resolved type in the context
// of one execution path.
type NodeTypeInfo struct {
	Tree *template.Tree // the template tree containing Node
	Node template.Node  // the node whose type was resolved
	Type types.Type
	// Object is the Go field or method a field/chain/variable node
	// resolves to (its last segment), or nil for nodes that don't name
	// one. Its Pos gives the definition site for go-to-definition.
	Object types.Object
	// Dot is the type of "." in effect where the node was checked —
	// narrowed inside {{with}}/{{range}} bodies. Completion enumerates
	// this type's members for the node's position.
	Dot types.Type
	// Vars are the template variables in lexical scope where the node was
	// checked, keyed by name including the leading "$" (so "$" itself, plus
	// any {{$x := ...}} / {{range $i, $v := ...}} declarations). Completion
	// offers these for a "$"-prefixed identifier at the node's position.
	Vars map[string]types.Type
}

// NodeInspectorFunc receives each node type as the checker resolves it.
type NodeInspectorFunc func(NodeTypeInfo)

func NewGlobal(pkg *types.Package, fileSet *token.FileSet, trees TreeFinder, fnChecker CallChecker) *Global {
	return &Global{
		trees:           trees,
		calls:           fnChecker,
		pkg:             pkg,
		fileSet:         fileSet,
		typeNodeMapping: make(TypeNodeMapping),
	}
}

// TypeString returns the string representation of typ using the configured Qualifier.
func (g *Global) TypeString(typ types.Type) string {
	var buf bytes.Buffer
	types.WriteType(&buf, typ, g.Qualifier)
	return buf.String()
}

// formatNotFoundParts returns both forms of the not-found message: bare
// (without the trailing "; available: ..." or "; no exported fields..." clause)
// and full (with that clause). Verbose error rendering uses the bare form
// because it follows up with a source declaration of the receiver type.
func (g *Global) formatNotFoundParts(ident string, tp types.Type) (bare, full string) {
	var b strings.Builder
	b.WriteString("field or method ")
	b.WriteString(ident)
	b.WriteString(" not found on ")
	b.WriteString(g.TypeString(tp))
	if named, ok := tp.(*types.Named); ok && g.fileSet != nil {
		pos := g.fileSet.Position(named.Obj().Pos())
		if pos.IsValid() {
			fmt.Fprintf(&b, " (declared at %s)", pos)
		}
	}
	bare = b.String()

	members := g.collectMembers(tp)
	if len(members) == 0 {
		full = bare + "; no exported fields or methods"
	} else {
		full = bare + "; available: " + strings.Join(members, ", ")
	}
	return bare, full
}

func (g *Global) collectMembers(tp types.Type) []string {
	var members []string
	seen := make(map[string]bool)

	// Collect methods via method set.
	// For interfaces, use the type directly (pointer-to-interface has empty method set).
	// For concrete types, use pointer to pick up pointer receiver methods.
	var mset *types.MethodSet
	if types.IsInterface(tp) {
		mset = types.NewMethodSet(tp)
	} else {
		mset = types.NewMethodSet(types.NewPointer(tp))
	}
	for i := range mset.Len() {
		obj := mset.At(i).Obj()
		name := obj.Name()
		if !token.IsExported(name) {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		sigStr := g.TypeString(obj.Type())
		// sigStr looks like "func(...) ..." — trim the "func" prefix to get "(...) ..."
		sigStr = strings.TrimPrefix(sigStr, "func")
		members = append(members, name+sigStr)
	}

	// Collect struct fields (including promoted fields from embedded structs).
	if tp != nil {
		if st, ok := tp.Underlying().(*types.Struct); ok {
			g.collectStructFields(st, seen, &members)
		}
	}

	slices.Sort(members)
	return members
}

// MemberKind distinguishes a struct field from a method.
type MemberKind int

const (
	MemberField MemberKind = iota
	MemberMethod
)

// Member is an exported field or method reachable on a type. It is the
// structured form of the member set the checker computes for its
// not-found error messages, exposed for completion tooling.
type Member struct {
	Name string
	Kind MemberKind
	Type string // field type, or method signature with the leading "func" trimmed
}

// Members returns the exported fields and methods reachable on tp
// (including promoted ones), sorted by name. It mirrors the traversal in
// collectMembers so completion and error messages agree on what exists.
// Pointer types are dereferenced first, so a pointer dot (e.g. from
// Execute with &T{}) reports T's members rather than nothing.
func Members(tp types.Type) []Member {
	if tp == nil {
		return nil
	}
	tp = dereference(tp)
	var out []Member
	seen := make(map[string]bool)

	var mset *types.MethodSet
	if types.IsInterface(tp) {
		mset = types.NewMethodSet(tp)
	} else {
		mset = types.NewMethodSet(types.NewPointer(tp))
	}
	for i := range mset.Len() {
		obj := mset.At(i).Obj()
		name := obj.Name()
		if !token.IsExported(name) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, Member{
			Name: name,
			Kind: MemberMethod,
			Type: strings.TrimPrefix(types.TypeString(obj.Type(), nil), "func"),
		})
	}
	if st, ok := tp.Underlying().(*types.Struct); ok {
		collectExportedFields(st, seen, &out)
	}
	slices.SortFunc(out, func(a, b Member) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func collectExportedFields(st *types.Struct, seen map[string]bool, out *[]Member) {
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		if f.Embedded() {
			if inner, ok := f.Type().Underlying().(*types.Struct); ok {
				collectExportedFields(inner, seen, out)
			}
			continue
		}
		if seen[f.Name()] {
			continue
		}
		seen[f.Name()] = true
		*out = append(*out, Member{Name: f.Name(), Kind: MemberField, Type: types.TypeString(f.Type(), nil)})
	}
}

func (g *Global) collectStructFields(st *types.Struct, seen map[string]bool, members *[]string) {
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		if f.Embedded() {
			if inner, ok := f.Type().Underlying().(*types.Struct); ok {
				g.collectStructFields(inner, seen, members)
			}
			continue
		}
		if seen[f.Name()] {
			continue
		}
		seen[f.Name()] = true
		*members = append(*members, f.Name()+" "+g.TypeString(f.Type()))
	}
}

// TreeFinder should wrap https://pkg.go.dev/html/template#Template.Lookup and return the Tree field from the Template
// If you are using text/template the lookup function from that package should also work.
type TreeFinder interface {
	FindTree(name string) (*template.Tree, bool)
}

type FindTreeFunc func(name string) (*template.Tree, bool)

func (fn FindTreeFunc) FindTree(name string) (*template.Tree, bool) {
	return fn(name)
}

type CallChecker interface {
	CheckCall(*Global, string, []template.Node, []types.Type) (types.Type, error)
}

type TypeNodeMapping map[types.Type][]template.Node

func Execute(global *Global, tree *template.Tree, data types.Type) error {
	// AST-only warning passes run independently of type checking, so they
	// still report even when the walk below fails.
	reportUnusedVariables(global, tree)
	s := newTemplateScope(global, tree, data, nil)
	_, err := s.walk(tree, data, nil, tree.Root)
	return err
}

// newTemplateScope returns the scope for walking tree's body with dot,
// invoked from within the templates in parent.
func newTemplateScope(global *Global, tree *template.Tree, dot types.Type, parent []templateInvocation) *scope {
	return &scope{
		global: global,
		variables: map[string]types.Type{
			"$": dot,
		},
		// Clip so sibling invocations never share a backing array.
		invocations: append(slices.Clip(parent), templateInvocation{tree: tree, dot: dot}),
	}
}

type scope struct {
	global    *Global
	variables map[string]types.Type
	// lastObject holds the Go object the most recent field/method
	// resolution landed on, for emit to attach to that node's
	// NodeTypeInfo. emit clears it, so it never leaks to another node.
	lastObject types.Object
	// guarded holds the dot-rooted field paths known non-nil in this scope
	// ("" is the dot itself, ".User" is dot.User), populated by enclosing
	// {{if}}/{{with}} conditions; it suppresses the W003 nil-pointer warning.
	guarded map[string]bool

	// invocations are the template trees being checked, outermost first,
	// each with the dot it was invoked with. A tree invoked again with an
	// identical dot is already being checked further up, so walking it
	// again would only repeat its errors, or never end when it recurses.
	// Trees are compared by identity, not name, so a root tree whose name
	// resolves to a different body does not hide that body.
	invocations []templateInvocation
}

type templateInvocation struct {
	tree *template.Tree
	dot  types.Type
}

func (s *scope) child() *scope {
	return &scope{
		global:      s.global,
		variables:   maps.Clone(s.variables),
		guarded:     maps.Clone(s.guarded),
		invocations: s.invocations,
	}
}

// invoking reports whether tree is already being checked with an
// identical dot.
func (s *scope) invoking(tree *template.Tree, dot types.Type) bool {
	return slices.ContainsFunc(s.invocations, func(inv templateInvocation) bool {
		return inv.tree == tree && types.Identical(inv.dot, dot)
	})
}

func (s *scope) walk(tree *template.Tree, dot, prev types.Type, node template.Node) (types.Type, error) {
	tp, err := s.walkNode(tree, dot, prev, node)
	return s.emit(tree, node, dot, tp, err)
}

// emit reports a resolved node type to Global.InspectNode (when set, the
// resolution succeeded, and a non-nil type was produced) and passes the
// (type, error) through unchanged, so it can wrap a resolver's return.
// Nodes that carry no type of their own (lists, text, control structures)
// resolve to a nil type and are not reported.
func (s *scope) emit(tree *template.Tree, node template.Node, dot, tp types.Type, err error) (types.Type, error) {
	obj := s.lastObject
	s.lastObject = nil
	if err == nil && tp != nil && s.global.InspectNode != nil {
		s.global.InspectNode(NodeTypeInfo{Tree: tree, Node: node, Type: tp, Object: obj, Dot: dot, Vars: maps.Clone(s.variables)})
	}
	return tp, err
}

func (s *scope) walkNode(tree *template.Tree, dot, prev types.Type, node template.Node) (types.Type, error) {
	switch n := node.(type) {
	case *template.DotNode:
		return dot, nil
	case *template.ListNode:
		return nil, s.checkListNode(tree, dot, prev, n)
	case *template.ActionNode:
		return nil, s.checkActionNode(tree, dot, prev, n)
	case *template.CommandNode:
		return s.checkCommandNode(tree, dot, prev, n)
	case *template.FieldNode:
		return s.checkFieldNode(tree, dot, n, nil)
	case *template.PipeNode:
		return s.checkPipeNode(tree, dot, n)
	case *template.IfNode:
		return nil, s.checkIfNode(tree, dot, n)
	case *template.RangeNode:
		return nil, s.checkRangeNode(tree, dot, n)
	case *template.TemplateNode:
		return nil, s.checkTemplateNode(tree, dot, n)
	case *template.BoolNode:
		return types.Typ[types.Bool], nil
	case *template.StringNode:
		return types.Typ[types.String], nil
	case *template.NumberNode:
		return newNumberNodeType(tree, n)
	case *template.VariableNode:
		return s.checkVariableNode(tree, n, nil)
	case *template.IdentifierNode:
		return s.checkIdentifierNode(tree, n)
	case *template.TextNode:
		return nil, nil
	case *template.WithNode:
		return nil, s.checkWithNode(tree, dot, n)
	case *template.CommentNode:
		return nil, nil
	case *template.NilNode:
		return types.Typ[types.UntypedNil], nil
	case *template.ChainNode:
		return s.checkChainNode(tree, dot, prev, n, nil)
	case *template.BreakNode:
		return nil, nil
	case *template.ContinueNode:
		return nil, nil
	// HTML-specific nodes
	case *template.HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			if _, err := s.walk(tree, dot, prev, child); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case *template.HTMLElementNode:
		for _, attr := range n.Attr {
			if _, err := s.walk(tree, dot, prev, attr); err != nil {
				return nil, err
			}
		}
		for _, child := range n.Children {
			if _, err := s.walk(tree, dot, prev, child); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case *template.HTMLAttributeNode:
		for _, k := range n.Key {
			if _, err := s.walk(tree, dot, prev, k); err != nil {
				return nil, err
			}
		}
		for _, v := range n.Value {
			if _, err := s.walk(tree, dot, prev, v); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case *template.HTMLCommentNode:
		for _, child := range n.Children {
			if _, err := s.walk(tree, dot, prev, child); err != nil {
				return nil, err
			}
		}
		return nil, nil
	case *template.HTMLDoctypeNode:
		return nil, nil
	case *template.DefineNode:
		return nil, nil
	case *template.BlockNode:
		return nil, s.checkBlockNode(tree, dot, prev, n)
	default:
		return nil, newError(tree, n, "missing node type check %T", n)
	}
}

func (s *scope) checkBlockNode(tree *template.Tree, dot, _ types.Type, n *template.BlockNode) error {
	x := dot
	if n.Pipe != nil {
		tp, err := s.walk(tree, x, nil, n.Pipe)
		if err != nil {
			return err
		}
		x = tp
		x = downgradeUntyped(x)
	}
	if n.Body != nil {
		childScope := scope{
			global: s.global,
			variables: map[string]types.Type{
				"$": x,
			},
			// The body is walked inline, so the templates enclosing the
			// block are still being checked.
			invocations: s.invocations,
		}
		if _, err := childScope.walk(tree, x, nil, n.Body); err != nil {
			return err
		}
	}
	return nil
}

func (s *scope) checkChainNode(tree *template.Tree, dot, prev types.Type, n *template.ChainNode, args []types.Type) (types.Type, error) {
	x, err := s.walk(tree, dot, prev, n.Node)
	if err != nil {
		return nil, err
	}
	return s.checkIdentifiers(tree, x, n, n.Field, args)
}

func (s *scope) checkVariableNode(tree *template.Tree, n *template.VariableNode, args []types.Type) (types.Type, error) {
	tp, ok := s.variables[n.Ident[0]]
	if !ok {
		return nil, newError(tree, n, "variable %s not found", n.Ident[0])
	}
	return s.checkIdentifiers(tree, tp, n, n.Ident[1:], args)
}

func (s *scope) checkListNode(tree *template.Tree, dot, prev types.Type, n *template.ListNode) error {
	for _, child := range n.Nodes {
		if _, err := s.walk(tree, dot, prev, child); err != nil {
			return err
		}
	}
	return nil
}

func (s *scope) checkActionNode(tree *template.Tree, dot, prev types.Type, n *template.ActionNode) error {
	_, err := s.walk(tree, dot, prev, n.Pipe)
	return err
}

func (s *scope) checkPipeNode(tree *template.Tree, dot types.Type, n *template.PipeNode) (types.Type, error) {
	var result types.Type
	for _, cmd := range n.Commands {
		tp, err := s.walk(tree, dot, result, cmd)
		if err != nil {
			return nil, err
		}
		result = tp
	}
	if len(n.Decl) > 0 && len(n.Decl[0].Ident) > 0 {
		s.variables[n.Decl[0].Ident[0]] = result
	}
	if len(n.Decl) > 1 && len(n.Decl[1].Ident) > 0 {
		s.variables[n.Decl[1].Ident[0]] = result
	}
	return result, nil
}

func (s *scope) checkIfNode(tree *template.Tree, dot types.Type, n *template.IfNode) error {
	_, err := s.walk(tree, dot, nil, n.Pipe)
	if err != nil {
		return err
	}
	if truth, ok := literalCondition(n.Pipe); ok {
		if truth && n.ElseList != nil && len(n.ElseList.Nodes) > 0 {
			s.warn(tree, n.ElseList, WarnDeadBranch, "{{else}} is never reached: the {{if}} condition is always true")
		} else if !truth && n.List != nil && len(n.List.Nodes) > 0 {
			s.warn(tree, n.List, WarnDeadBranch, "this {{if}} body is never reached: the condition is always false")
		}
	}
	ifScope := s.child()
	// A truthy {{if}} condition proves its subject non-nil for the body, so
	// dereferencing that pointer inside is safe (W003).
	if path, ok := guardedPath(n.Pipe); ok {
		if ifScope.guarded == nil {
			ifScope.guarded = make(map[string]bool)
		}
		ifScope.guarded[path] = true
	}
	if _, err := ifScope.walk(tree, dot, nil, n.List); err != nil {
		return err
	}
	if n.ElseList != nil {
		elseScope := s.child()
		if _, err := elseScope.walk(tree, dot, nil, n.ElseList); err != nil {
			return err
		}
	}
	return nil
}

func (s *scope) checkWithNode(tree *template.Tree, dot types.Type, n *template.WithNode) error {
	child := s.child()
	x, err := child.walk(tree, dot, nil, n.Pipe)
	if err != nil {
		return err
	}
	withScope := child.child()
	// {{with}} enters its body only when the subject is non-empty, so the
	// body dot is guaranteed non-nil — dereferencing it is safe (W003).
	if withScope.guarded == nil {
		withScope.guarded = make(map[string]bool)
	}
	withScope.guarded[""] = true
	if _, err := withScope.walk(tree, x, nil, n.List); err != nil {
		return err
	}
	if n.ElseList != nil {
		elseScope := child.child()
		if _, err := elseScope.walk(tree, dot, nil, n.ElseList); err != nil {
			return err
		}
	}
	return nil
}

func newNumberNodeType(tree *template.Tree, constant *template.NumberNode) (types.Type, error) {
	switch {
	case constant.IsComplex:
		return types.Typ[types.UntypedComplex], nil

	case constant.IsFloat &&
		!isHexInt(constant.Text) && !isRuneInt(constant.Text) &&
		strings.ContainsAny(constant.Text, ".eEpP"):
		return types.Typ[types.UntypedFloat], nil

	case constant.IsInt:
		n := int(constant.Int64)
		if int64(n) != constant.Int64 {
			return nil, newError(tree, constant, "%s overflows int", constant.Text)
		}
		return types.Typ[types.UntypedInt], nil

	case constant.IsUint:
		return nil, newError(tree, constant, "%s overflows int", constant.Text)
	}
	return types.Typ[types.UntypedInt], nil
}

func isRuneInt(s string) bool {
	return len(s) > 0 && s[0] == '\''
}

func isHexInt(s string) bool {
	return len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') && !strings.ContainsAny(s, "pP")
}

func (s *scope) checkTemplateNode(tree *template.Tree, dot types.Type, n *template.TemplateNode) error {
	x := dot
	if n.Pipe != nil {
		tp, err := s.walk(tree, x, nil, n.Pipe)
		if err != nil {
			return err
		}
		x = tp
		x = downgradeUntyped(x)
	} else {
		x = types.Typ[types.UntypedNil]
	}
	if fn := s.global.InspectTemplateNode; fn != nil {
		fn(n, tree, x)
	}
	childTree, ok := s.global.trees.FindTree(n.Name)
	if !ok {
		return newError(tree, n, "template %q not found", n.Name)
	}
	if s.invoking(childTree, x) {
		return nil
	}
	childScope := newTemplateScope(s.global, childTree, x, s.invocations)
	_, err := childScope.walk(childTree, x, nil, childTree.Root)
	return err
}

func downgradeUntyped(x types.Type) types.Type {
	if x == nil {
		return x
	}
	basic, ok := x.Underlying().(*types.Basic)
	if !ok {
		return x
	}
	switch k := basic.Kind(); k {
	case types.UntypedBool:
		return types.Typ[types.Bool].Underlying()
	case types.UntypedInt:
		return types.Typ[types.Int].Underlying()
	case types.UntypedRune:
		return types.Typ[types.Rune].Underlying()
	case types.UntypedFloat:
		return types.Typ[types.Float64].Underlying()
	case types.UntypedComplex:
		return types.Typ[types.Complex128].Underlying()
	case types.UntypedString:
		return types.Typ[types.String].Underlying()
	case types.UntypedNil:
		return types.Typ[types.UntypedNil]
	default:
		return x
	}
}

func (s *scope) checkFieldNode(tree *template.Tree, dot types.Type, n *template.FieldNode, args []types.Type) (types.Type, error) {
	return s.checkIdentifiers(tree, dot, n, n.Ident, args)
}

func (s *scope) checkCommandNode(tree *template.Tree, dot, prev types.Type, cmd *template.CommandNode) (types.Type, error) {
	first := cmd.Args[0]
	switch n := first.(type) {
	case *template.FieldNode:
		argTypes, err := s.argumentTypes(tree, dot, prev, cmd.Args[1:])
		if err != nil {
			return nil, err
		}
		tp, err := s.checkFieldNode(tree, dot, n, argTypes)
		return s.emit(tree, n, dot, tp, err)
	case *template.ChainNode:
		argTypes, err := s.argumentTypes(tree, dot, prev, cmd.Args[1:])
		if err != nil {
			return nil, err
		}
		tp, err := s.checkChainNode(tree, dot, prev, n, argTypes)
		return s.emit(tree, n, dot, tp, err)
	case *template.IdentifierNode:
		argFn := s.argumentTypes
		if n.Ident == "and" {
			argFn = s.argumentTypesGuarded // and short-circuits; guard later operands
		}
		argTypes, err := argFn(tree, dot, prev, cmd.Args[1:])
		if err != nil {
			return nil, err
		}
		tp, err := s.global.calls.CheckCall(s.global, n.Ident, cmd.Args[1:], argTypes)
		if err != nil {
			return nil, wrapError(tree, cmd, err)
		}
		return s.emit(tree, n, dot, tp, nil)
	case *template.PipeNode:
		if err := s.notAFunction(tree, n, cmd.Args, prev); err != nil {
			return nil, err
		}
		tp, err := s.checkPipeNode(tree, dot, n)
		return s.emit(tree, n, dot, tp, err)
	case *template.VariableNode:
		argTypes, err := s.argumentTypes(tree, dot, prev, cmd.Args[1:])
		if err != nil {
			return nil, err
		}
		tp, err := s.checkVariableNode(tree, n, argTypes)
		return s.emit(tree, n, dot, tp, err)
	}

	if err := s.notAFunction(tree, first, cmd.Args, prev); err != nil {
		return nil, err
	}

	switch n := first.(type) {
	case *template.BoolNode:
		return types.Typ[types.UntypedBool], nil
	case *template.StringNode:
		return types.Typ[types.UntypedString], nil
	case *template.NumberNode:
		return newNumberNodeType(tree, n)
	case *template.DotNode:
		return s.emit(tree, n, dot, dot, nil)
	case *template.NilNode:
		return nil, newError(tree, n, "nil is not a command")
	default:
		return nil, newError(tree, first, "can't evaluate command %q", first)
	}
}

func (s *scope) addGuard(path string) {
	if s.guarded == nil {
		s.guarded = make(map[string]bool)
	}
	s.guarded[path] = true
}

// argumentTypesGuarded checks the arguments of a built-in and left to right,
// treating each field-path operand as non-nil for the operands after it —
// and short-circuits, so {{if and .User (eq .User.Role "x")}} never
// dereferences a nil .User (W003).
func (s *scope) argumentTypesGuarded(tree *template.Tree, dot, prev types.Type, args []template.Node) ([]types.Type, error) {
	child := s.child()
	argTypes := make([]types.Type, 0, len(args)+1)
	for _, arg := range args {
		argType, err := child.walk(tree, dot, prev, arg)
		if err != nil {
			return nil, err
		}
		argTypes = append(argTypes, argType)
		if path, ok := fieldPathOf(arg); ok {
			child.addGuard(path)
		}
	}
	if prev != nil {
		argTypes = append(argTypes, prev)
	}
	return argTypes, nil
}

func (s *scope) argumentTypes(tree *template.Tree, dot types.Type, prev types.Type, args []template.Node) ([]types.Type, error) {
	argTypes := make([]types.Type, 0, len(args)+1)
	for _, arg := range args {
		argType, err := s.walk(tree, dot, prev, arg)
		if err != nil {
			return nil, err
		}
		argTypes = append(argTypes, argType)
	}
	if prev != nil {
		argTypes = append(argTypes, prev)
	}
	return argTypes, nil
}

func (s *scope) notAFunction(tree *template.Tree, node template.Node, args []template.Node, final types.Type) error {
	if len(args) > 1 || final != nil {
		return newError(tree, node, "can't give argument to non-function %s", args[0])
	}
	return nil
}

// identErr builds an *Error wrapping an *IdentifierError. The location
// prefix is added by *Error at format time; the IdentifierError carries
// the full type so callers of FormatVerbose can render its structure.
func (s *scope) identErr(tree *template.Tree, n template.Node, ident string, tp types.Type, format string, a ...any) *Error {
	return wrapError(tree, n, &IdentifierError{
		Identifier: ident,
		Type:       tp,
		Cause:      fmt.Errorf(format, a...),
		qualifier:  s.global.Qualifier,
		fset:       s.global.fileSet,
	})
}

func (s *scope) checkIdentifiers(tree *template.Tree, dot types.Type, n template.Node, idents []string, args []types.Type) (types.Type, error) {
	x := dot
	warnedInterface := false
	warnedNil := false
	prevFieldNonil := false
	for i, ident := range idents {
		// Reset per segment: only a real field/method resolution below
		// sets lastObject, so after the loop it reflects the last segment
		// exactly (or nil for a map index or other non-declared segment).
		s.lastObject = nil
		// W003: accessing a field dereferences the receiver, which panics at
		// runtime if it is a nil pointer. Warn (once per chain) unless the
		// pointer is guarded by an enclosing {{if}}/{{with}} or a
		// templatecheck:"nonil" struct tag.
		if !warnedNil && x != nil {
			if ptr, ok := x.Underlying().(*types.Pointer); ok {
				if _, isStruct := ptr.Elem().Underlying().(*types.Struct); isStruct {
					// Only a struct FIELD access is flagged: a method call
					// through a pointer may be written to be nil-safe (a
					// pointer receiver that checks for nil), so warning on it
					// would be noise.
					obj, _, _ := types.LookupFieldOrMethod(x, true, s.global.pkg, ident)
					if _, isField := obj.(*types.Var); isField {
						path := ""
						if i > 0 {
							path = "." + strings.Join(idents[:i], ".")
						}
						if !s.guarded[path] && !(i > 0 && prevFieldNonil) {
							s.warn(tree, n, WarnUnguardedPointer, "%s dereferences %s, which may be nil; guard it with {{with}} or {{if}}, or tag the field templatecheck:\"nonil\"", ident, s.global.TypeString(x))
							warnedNil = true
						}
					}
				}
			}
		}
		prevFieldNonil = false
		x = dereference(x)
		switch xx := x.(type) {
		case *types.Map:
			switch key := xx.Key().Underlying().(type) {
			case *types.Basic:
				switch key.Kind() {
				case types.Int:
					x = xx.Elem()
					_, err := strconv.Atoi(ident)
					if err != nil {
						return nil, s.identErr(tree, n, ident, xx, `can't evaluate field one in type %s`, s.global.TypeString(xx))
					}
				case types.String:
					x = xx.Elem()
				default:
				}
				continue
			default:
				x = xx.Elem()
			}
			continue
		default:
			if !token.IsExported(ident) {
				return nil, s.identErr(tree, n, ident, x, "field or method %s is not exported", ident)
			}
			obj, _, _ := types.LookupFieldOrMethod(x, true, s.global.pkg, ident)
			if obj == nil {
				// Accessing a field on an interface value cannot be verified
				// statically: warn (once per chain) and continue as an
				// unknown value, matching how the template resolves it at
				// runtime — rather than reporting a not-found error.
				if types.IsInterface(x) {
					if !warnedInterface {
						s.warn(tree, n, WarnInterfaceFieldAccess, "%s is accessed on interface value %s and cannot be checked until runtime", ident, s.global.TypeString(x))
						warnedInterface = true
					}
					x = emptyInterface
					continue
				}
				bare, full := s.global.formatNotFoundParts(ident, x)
				return nil, wrapError(tree, n, &IdentifierError{
					Identifier: ident,
					Type:       x,
					Cause:      errors.New(full),
					bareCause:  bare,
					qualifier:  s.global.Qualifier,
					fset:       s.global.fileSet,
				})
			}
			// Remember the object this segment resolves to; the last one
			// wins and is the field/method the whole node names.
			s.lastObject = obj
			// If this field is a pointer tagged templatecheck:"nonil", the
			// next segment may dereference it without a W003 warning.
			if v, isVar := obj.(*types.Var); isVar {
				if _, isPtr := v.Type().Underlying().(*types.Pointer); isPtr {
					prevFieldNonil = structFieldHasNonil(x, ident)
				}
			}
			switch o := obj.(type) {
			default:
				x = obj.Type()
			case *types.Func:
				sig := o.Signature()
				resultLen := sig.Results().Len()
				if resultLen < 1 || resultLen > 2 {
					methodPos := s.global.fileSet.Position(o.Pos())
					return nil, s.identErr(tree, n, ident, sig, "function %s has %d return values; should be 1 or 2: incorrect signature at %s", ident, resultLen, methodPos)
				}
				if resultLen > 1 {
					methodPos := s.global.fileSet.Position(obj.Pos())
					finalResult := sig.Results().At(sig.Results().Len() - 1)
					errorType := types.Universe.Lookup("error")
					if !types.Identical(errorType.Type(), finalResult.Type()) {
						return nil, s.identErr(tree, n, ident, sig, "invalid function signature for %s: second return value should be error; is %s: incorrect signature at %s", ident, s.global.TypeString(finalResult.Type()), methodPos)
					}
				}
				if i == len(idents)-1 {
					res, err := checkCallArguments(s.global, ident, sig, args)
					if err != nil {
						return nil, wrapError(tree, n, err)
					}
					return res, nil
				}
				x = sig.Results().At(0).Type()
			}
			if _, ok := x.(*types.Signature); ok && i < len(idents)-1 {
				return nil, s.identErr(tree, n, ident, x, "identifier chain not supported for type %s", s.global.TypeString(x))
			}
		}
	}
	if len(args) > 0 {
		sig, ok := x.(*types.Signature)
		if !ok {
			return nil, s.identErr(tree, n, "", x, "expected method or function")
		}
		var name string
		if len(idents) > 0 {
			name = idents[len(idents)-1]
		}
		tp, err := checkCallArguments(s.global, name, sig, args)
		if err != nil {
			return nil, wrapError(tree, n, err)
		}
		return tp, nil
	}
	return x, nil
}

func (s *scope) checkRangeNode(tree *template.Tree, dot types.Type, n *template.RangeNode) error {
	child := s.child()
	pipeType, err := child.walk(tree, dot, nil, n.Pipe)
	if err != nil {
		return err
	}
	pipeType = dereference(pipeType).Underlying()
	var x types.Type
	switch pt := pipeType.(type) {
	case *types.Slice:
		x = pt.Elem()
		if len(n.Pipe.Decl) == 1 {
			child.variables[n.Pipe.Decl[0].Ident[0]] = x
		} else if len(n.Pipe.Decl) > 1 {
			child.variables[n.Pipe.Decl[0].Ident[0]] = types.Typ[types.Int]
			child.variables[n.Pipe.Decl[1].Ident[0]] = x
		}
	case *types.Array:
		x = pt.Elem()
		if len(n.Pipe.Decl) == 1 {
			child.variables[n.Pipe.Decl[0].Ident[0]] = x
		} else if len(n.Pipe.Decl) > 1 {
			child.variables[n.Pipe.Decl[0].Ident[0]] = types.Typ[types.Int]
			child.variables[n.Pipe.Decl[1].Ident[0]] = x
		}
	case *types.Map:
		x = pt.Elem()
		if len(n.Pipe.Decl) == 1 {
			child.variables[n.Pipe.Decl[0].Ident[0]] = pt.Elem()
		} else if len(n.Pipe.Decl) > 1 {
			child.variables[n.Pipe.Decl[0].Ident[0]] = pt.Key()
			child.variables[n.Pipe.Decl[1].Ident[0]] = pt.Elem()
		}
	case *types.Chan:
		x = pt.Elem()
		if len(n.Pipe.Decl) == 1 {
			child.variables[n.Pipe.Decl[0].Ident[0]] = pt.Elem()
		} else if len(n.Pipe.Decl) > 1 {
			child.variables[n.Pipe.Decl[0].Ident[0]] = types.Typ[types.Int]
			child.variables[n.Pipe.Decl[1].Ident[0]] = pt.Elem()
		}
	case *types.Basic:
		switch {
		case pt.Info()&types.IsInteger != 0:
			tp := types.Universe.Lookup(strings.TrimPrefix(pipeType.String(), "untyped "))
			x = tp.Type()
			if len(n.Pipe.Decl) > 0 {
				child.variables[n.Pipe.Decl[0].Ident[0]] = x
			}
		default:
			return newError(tree, n.Pipe, "range can't iterate over %s", strings.TrimPrefix(s.global.TypeString(pipeType), "untyped "))
		}
	case *types.Signature:
		if v1, v2, ok := isIter2(pt); ok {
			// Two range variables bind key then value, and dot is the value
			// — matching text/template ({{range $k, $v := seq2}} sets . to
			// the value). With one (or no) variable dot is the key.
			if len(n.Pipe.Decl) > 1 {
				x = v2
				child.variables[n.Pipe.Decl[0].Ident[0]] = v1
				child.variables[n.Pipe.Decl[1].Ident[0]] = v2
			} else {
				x = v1
				if len(n.Pipe.Decl) > 0 {
					child.variables[n.Pipe.Decl[0].Ident[0]] = v1
				}
			}
		} else if val, ok := isIter(pt); ok {
			x = val
			if len(n.Pipe.Decl) == 1 {
				child.variables[n.Pipe.Decl[0].Ident[0]] = val
			}
			if len(n.Pipe.Decl) > 1 {
				return newError(tree, n.Pipe, "iter.Seq[T] must not iterate over more than one variable")
			}
		} else {
			return newError(tree, n.Pipe, "failed to range over function %s", s.global.TypeString(pipeType))
		}
	default:
		return newError(tree, n.Pipe, "failed to range over %s", s.global.TypeString(pipeType))
	}
	// A range element — the body dot and the loop variables — may be a
	// pointer, but flagging every {{range .Ptrs}}{{.Field}} would be noise:
	// the common case has no nil elements. Treat the element as guarded for
	// the W003 nil-pointer warning.
	if child.guarded == nil {
		child.guarded = make(map[string]bool)
	}
	child.guarded[""] = true
	if _, err := child.walk(tree, x, nil, n.List); err != nil {
		return err
	}
	if n.ElseList != nil {
		if _, err := child.walk(tree, x, nil, n.ElseList); err != nil {
			return err
		}
	}
	return nil
}

func isIter(signature *types.Signature) (types.Type, bool) {
	if signature == nil || signature.Variadic() || signature.Results().Len() != 0 || signature.Params().Len() != 1 {
		return nil, false
	}
	yield, ok := signature.Params().At(0).Type().(*types.Signature)
	if !ok || yield.Results().Len() != 1 || yield.Params().Len() != 1 {
		return nil, false
	}
	if !types.Identical(yield.Results().At(0).Type(), types.Universe.Lookup("bool").Type()) {
		return nil, false
	}
	return yield.Params().At(0).Type(), true
}

func isIter2(signature *types.Signature) (types.Type, types.Type, bool) {
	if signature == nil || signature.Variadic() || signature.Results().Len() != 0 || signature.Params().Len() != 1 {
		return nil, nil, false
	}
	yield, ok := signature.Params().At(0).Type().(*types.Signature)
	if !ok || yield.Results().Len() != 1 || yield.Params().Len() != 2 {
		return nil, nil, false
	}
	if !types.Identical(yield.Results().At(0).Type(), types.Universe.Lookup("bool").Type()) {
		return nil, nil, false
	}
	yp := yield.Params()
	return yp.At(0).Type(), yp.At(1).Type(), true
}

func (s *scope) checkIdentifierNode(tree *template.Tree, n *template.IdentifierNode) (types.Type, error) {
	if !strings.HasPrefix(n.Ident, "$") {
		tp, err := s.global.calls.CheckCall(s.global, n.Ident, nil, nil)
		if err != nil {
			return nil, wrapError(tree, n, err)
		}
		return tp, err
	}
	tp, ok := s.variables[n.Ident]
	if !ok {
		return nil, newError(tree, n, "failed to find identifier %s", n.Ident)
	}
	return tp, nil
}

func dereference(tp types.Type) types.Type {
	for {
		ptr, ok := tp.(*types.Pointer)
		if !ok {
			return tp
		}
		tp = ptr.Elem()
	}
}
