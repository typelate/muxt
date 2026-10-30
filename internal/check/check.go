package check

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// ErrorType classifies the failure a *Error reports. It lets tools such as
// language servers map errors to stable diagnostic codes without parsing
// error messages.
type ErrorType int

const (
	// ErrorTypeUnknown marks errors not classified by this package, such as
	// those returned by a custom CallChecker implementation.
	ErrorTypeUnknown ErrorType = iota
	// ErrorTypeAggregate marks an error that only groups child errors.
	ErrorTypeAggregate
	// ErrorTypeUnexpectedNode reports a parse node the checker does not know
	// how to walk.
	ErrorTypeUnexpectedNode
	// ErrorTypeVariableNotFound reports use of an undeclared template variable.
	ErrorTypeVariableNotFound
	// ErrorTypeConstantOverflow reports a numeric constant that overflows int.
	ErrorTypeConstantOverflow
	// ErrorTypeTemplateNotFound reports invoking an undefined associated template.
	ErrorTypeTemplateNotFound
	// ErrorTypeBadCommand reports a command that cannot be evaluated, such as nil.
	ErrorTypeBadCommand
	// ErrorTypeNotAFunction reports arguments given to a non-function value.
	ErrorTypeNotAFunction
	// ErrorTypeFieldNotExported reports access to an unexported field or method.
	ErrorTypeFieldNotExported
	// ErrorTypeFieldOrMethodNotFound reports a field or method lookup failure.
	ErrorTypeFieldOrMethodNotFound
	// ErrorTypeBadSignature reports a method or function whose signature is
	// not callable from a template.
	ErrorTypeBadSignature
	// ErrorTypeCallArguments reports a call with the wrong number or types of
	// arguments.
	ErrorTypeCallArguments
	// ErrorTypeUnknownFunction reports a call to an undefined function.
	ErrorTypeUnknownFunction
	// ErrorTypeRange reports a range over a type that cannot be iterated.
	ErrorTypeRange
	// ErrorTypeIdentifierChain reports a field chain through a type that does
	// not support further selection.
	ErrorTypeIdentifierChain
	// ErrorTypeMapKey reports a map index whose key cannot match the map's
	// key type.
	ErrorTypeMapKey
)

// String returns a stable slug for the error type, suitable for use as a
// diagnostic code.
func (t ErrorType) String() string {
	switch t {
	case ErrorTypeAggregate:
		return "aggregate"
	case ErrorTypeUnexpectedNode:
		return "unexpected-node"
	case ErrorTypeVariableNotFound:
		return "variable-not-found"
	case ErrorTypeConstantOverflow:
		return "constant-overflow"
	case ErrorTypeTemplateNotFound:
		return "template-not-found"
	case ErrorTypeBadCommand:
		return "bad-command"
	case ErrorTypeNotAFunction:
		return "not-a-function"
	case ErrorTypeFieldNotExported:
		return "field-not-exported"
	case ErrorTypeFieldOrMethodNotFound:
		return "field-or-method-not-found"
	case ErrorTypeBadSignature:
		return "bad-signature"
	case ErrorTypeCallArguments:
		return "call-arguments"
	case ErrorTypeUnknownFunction:
		return "unknown-function"
	case ErrorTypeRange:
		return "range"
	case ErrorTypeIdentifierChain:
		return "identifier-chain"
	case ErrorTypeMapKey:
		return "map-key"
	default:
		return "unknown"
	}
}

// Error is a check failure located in a template, or an aggregate of
// several independent failures. The single-line format of a leaf is
//
//	{file}:{line}:{col}: executing {tree-name} at <{node-text}>: {message}
//
// The leading file:line:col is recognized by terminals and IDEs as a
// jump-to-source location.
type Error struct {
	// Type classifies the failure.
	Type ErrorType

	Tree *template.Tree
	Node template.Node

	// X is the type most relevant to the failure: the receiver for field or
	// method lookups, the pipeline result for range, the signature for call
	// errors. It is nil when no type is relevant.
	X types.Type

	// Decl is the source position where the Go declaration involved in the
	// failure is defined: the receiver type for field or method lookups,
	// the method for signature failures. It is the zero value when no
	// declaration position is known.
	Decl token.Position

	// Secondary marks a follow-on failure whose root cause is another error
	// in the same tree: a variable lookup that failed only because the
	// pipeline declaring that variable already failed. Diagnostic tools may
	// suppress or de-emphasize secondary errors.
	Secondary bool

	err error

	// render re-renders the cause message with a caller-chosen type
	// formatter. It is set when the message embeds type names; nil means
	// the message has no types to re-render.
	render func(typeFormatFunc) string

	// children holds the child errors when this error aggregates several
	// independent failures found while walking the same subtree.
	children []*Error
}

func newError(errType ErrorType, tree *template.Tree, node template.Node, message string, args ...any) *Error {
	e := errorf(errType, message, args...)
	e.Tree = tree
	e.Node = node
	return e
}

// errorf builds a located-later *Error: the walk site that receives it fills
// in Tree and Node via wrapError. types.Type args are rendered with a nil
// qualifier (full package paths) in the Error message and re-rendered with
// the caller's qualifier by DetailedError.
func errorf(errType ErrorType, message string, args ...any) *Error {
	render := renderer(message, args...)
	return &Error{
		Type:   errType,
		err:    errors.New(render(fullTypeFormat(nil))),
		render: render,
	}
}

// withX sets the type most relevant to the failure and returns e.
func (e *Error) withX(x types.Type) *Error {
	e.X = x
	return e
}

// withDecl sets the position of the involved Go declaration and returns e.
func (e *Error) withDecl(pos token.Position) *Error {
	e.Decl = pos
	return e
}

// wrapError locates err at node. When err is already a *Error, its missing
// location and classification are filled in on a copy; otherwise err becomes
// the cause of a new leaf error.
func wrapError(errType ErrorType, tree *template.Tree, node template.Node, err error) *Error {
	if e, ok := err.(*Error); ok {
		located := *e
		if located.Tree == nil {
			located.Tree = tree
		}
		if located.Node == nil {
			located.Node = node
		}
		if located.Type == ErrorTypeUnknown {
			located.Type = errType
		}
		return &located
	}
	return &Error{
		Type: errType,
		Tree: tree,
		Node: node,
		err:  err,
	}
}

// Error returns the single-line error message. The format is
//
//	{file}:{line}:{col}: executing {tree-name} at <{node-text}>: {message}
//
// The leading file:line:col is recognized by terminals and IDEs as a
// jump-to-source location. The message after the location matches the
// shape produced by text/template at runtime. An aggregate joins the
// messages of its children with newlines.
func (e *Error) Error() string {
	if len(e.children) > 0 {
		messages := make([]string, len(e.children))
		for i, child := range e.children {
			messages[i] = child.Error()
		}
		return strings.Join(messages, "\n")
	}
	return e.line(fullTypeFormat(nil))
}

// typeFormatFunc renders a types.Type to a string. Error uses
// fullTypeFormat(nil); DetailedError uses a qualifying, eliding formatter.
type typeFormatFunc func(types.Type) string

// fullTypeFormat renders complete type strings qualified with q.
func fullTypeFormat(q types.Qualifier) typeFormatFunc {
	return func(tp types.Type) string {
		return formatType(tp, q)
	}
}

// line renders the single-line message for a leaf error, rendering type
// names with tf.
func (e *Error) line(tf typeFormatFunc) string {
	return e.locationPrefix() + e.messageWith(tf)
}

// locationPrefix returns the "{file}:{line}:{col}: executing {tree-name}
// at <{node-text}>: " prefix of a located error, or "" when the error has
// no tree or node.
func (e *Error) locationPrefix() string {
	if e.Tree == nil || e.Node == nil {
		return ""
	}
	pos := e.Tree.Position(e.Node)
	loc := cmp.Or(e.Tree.Filepath, pos.ParseName)
	return fmt.Sprintf("%s:%d:%d: executing %q at <%s>: ",
		loc, pos.Line, pos.Column+chainColumnOffset(e.Node), e.Tree.Name, pos.Source)
}

// chainColumnOffset is how far into node text/template's parser places a
// chained field or variable node: at the field after the first segment,
// where the chain begins, rather than at the node's start. A chain has no
// space or newline between its segments, so the offset is a column count.
// Position and EndPosition keep spanning the whole node.
func chainColumnOffset(node template.Node) int {
	switch n := node.(type) {
	case *template.FieldNode:
		if len(n.Ident) > 1 {
			return len(".") + len(n.Ident[0])
		}
	case *template.VariableNode:
		if len(n.Ident) > 1 {
			return len(n.Ident[0])
		}
	}
	return 0
}

// messageWith renders the cause message through tf when the error (or its
// cause) recorded how to re-render its message.
func (e *Error) messageWith(tf typeFormatFunc) string {
	if e.render != nil {
		return e.render(tf)
	}
	if identErr, ok := errors.AsType[*IdentifierError](e.err); ok && identErr.render != nil {
		return identErr.render(tf)
	}
	if callErr, ok := errors.AsType[*CallError](e.err); ok && callErr.render != nil {
		return callErr.render(tf)
	}
	if builtinErr, ok := errors.AsType[*BuiltinError](e.err); ok && builtinErr.render != nil {
		return builtinErr.render(tf)
	}
	return e.err.Error()
}

// untypedTrimmed wraps a type whose message drops the "untyped " prefix
// go/types gives constant types, so a range over 1.2 reports "float" rather
// than "untyped float". Wrapping keeps the type itself in the argument list
// so DetailedError can still re-render it with its own qualifier.
type untypedTrimmed struct{ tp types.Type }

// renderFormat is fmt.Sprintf with types.Type args rendered through tf.
func renderFormat(tf typeFormatFunc, format string, args ...any) string {
	rendered := make([]any, len(args))
	for i, arg := range args {
		switch a := arg.(type) {
		case untypedTrimmed:
			rendered[i] = strings.TrimPrefix(tf(a.tp), "untyped ")
		case types.Type:
			rendered[i] = tf(a)
		default:
			rendered[i] = arg
		}
	}
	return fmt.Sprintf(format, rendered...)
}

// renderer captures format and args for re-rendering with any type
// formatter.
func renderer(format string, args ...any) func(typeFormatFunc) string {
	return func(tf typeFormatFunc) string {
		return renderFormat(tf, format, args...)
	}
}

// Unwrap exposes the underlying errors: the wrapped cause for a leaf error,
// or the child errors for an aggregate.
func (e *Error) Unwrap() []error {
	errs := make([]error, 0, len(e.children)+1)
	if e.err != nil {
		errs = append(errs, e.err)
	}
	for _, child := range e.children {
		errs = append(errs, child)
	}
	return errs
}

// All is an iter.Seq[*Error] that walks the error tree in depth-first
// pre-order, yielding e itself first and then the subtree of each child:
//
//	for err := range checkErr.All { ... }
func (e *Error) All(yield func(*Error) bool) {
	e.walkAll(yield)
}

func (e *Error) walkAll(yield func(*Error) bool) bool {
	if !yield(e) {
		return false
	}
	for _, child := range e.children {
		if !child.walkAll(yield) {
			return false
		}
	}
	return true
}

// joinErrors combines the non-nil errors found while walking node into a
// single error: nil when there are none, the error itself when there is one,
// and an aggregate *Error otherwise. Errors that are not *Error are promoted
// to leaf errors located at node.
func joinErrors(tree *template.Tree, node template.Node, errs ...error) error {
	var children []*Error
	for _, err := range errs {
		if err == nil {
			continue
		}
		child, ok := err.(*Error)
		if !ok {
			child = wrapError(ErrorTypeUnknown, tree, node, err)
		}
		children = append(children, child)
	}
	switch len(children) {
	case 0:
		return nil
	case 1:
		return children[0]
	default:
		return &Error{Type: ErrorTypeAggregate, Tree: tree, Node: node, children: children}
	}
}

// Position returns the start of the error's node as a token.Position: the
// template file (its ParseName when it has no file path), the byte Offset,
// and the 1-based Line/Column. Tooling uses it — together with EndPosition
// and the position-free message from Unwrap — to place a diagnostic without
// parsing the Error string's location prefix. It is the zero Position when
// the error has no tree or node, as an aggregate of a whole package may not.
func (e *Error) Position() token.Position {
	if e.Tree == nil || e.Node == nil {
		return token.Position{}
	}
	return e.positionAt(e.Node.Start())
}

// EndPosition returns the exclusive end of the error's node, the companion
// to Position for a ranged diagnostic.
func (e *Error) EndPosition() token.Position {
	if e.Tree == nil || e.Node == nil {
		return token.Position{}
	}
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
// stays empty rather than becoming a whitespace-only "  " line. An
// aggregate renders one block per child, separated by a blank line.
func (e *Error) VerboseError() string {
	if len(e.children) > 0 {
		blocks := make([]string, len(e.children))
		for i, child := range e.children {
			blocks[i] = child.VerboseError()
		}
		return strings.Join(blocks, "\n\n")
	}
	prefix := e.locationPrefix()
	v, ok := errors.AsType[VerboseErrorer](e.err)
	if !ok {
		return prefix + e.err.Error()
	}
	lines := strings.Split(v.VerboseError(), "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = "  " + lines[i]
		}
	}
	return prefix + strings.Join(lines, "\n")
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

	// WarnInterfaceFieldAccess, when true, turns a field or method looked
	// up on an interface value from an error into a W004 warning: the
	// lookup cannot be checked until runtime, so the checker continues
	// with an unknown value. When false, the default, such a lookup is
	// a "field or method ... not found" error, as upstream reports it.
	WarnInterfaceFieldAccess bool

	// Definitions locates where each template was defined, for the
	// inspector callbacks. It may be nil, in which case inspectors
	// receive a zero Definition.
	Definitions DefinitionFinder

	// Qualifier controls how types are printed by TypeString and by the
	// legacy VerboseError/FormatVerbose rendering only. Error messages
	// always print full package paths (nil qualifier), and DetailedError
	// takes its own types.Qualifier parameter — prefer that for qualified
	// rendering. See types.WriteType for qualifier semantics.
	Qualifier types.Qualifier
}

// TemplateNodeInspectorFunc is called for each {{template}} node whose
// pipeline type checks.
//
// The tree is the one holding node, the tree the node was written in, not
// the tree of the template it invokes; use it to locate node, for example
// with its Position method. The type is the dot flowing into the invoked
// template, and def locates where that template was defined. A node
// naming a template that does not exist still reaches the inspector, with
// a zero def.
type TemplateNodeInspectorFunc func(node *template.TemplateNode, t *template.Tree, tp types.Type, def Definition)

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

// definition reports where name was defined, or the zero Definition when
// no definition source is configured or the name is unknown.
func (g *Global) definition(name string) Definition {
	if g.Definitions == nil {
		return Definition{}
	}
	def, ok := g.Definitions.FindDefinition(name)
	if !ok {
		return Definition{}
	}
	return def
}

// TypeString returns the string representation of typ using the configured Qualifier.
func (g *Global) TypeString(typ types.Type) string {
	var buf bytes.Buffer
	types.WriteType(&buf, typ, g.Qualifier)
	return buf.String()
}

// notFoundMessage returns a renderer for the compact single-line not-found
// message. The exported members of tp are not listed here (DetailedError
// renders them), and the declaration position is not embedded (the Error's
// Decl field carries it), keeping the message deterministic.
func notFoundMessage(ident string, tp types.Type) func(typeFormatFunc) string {
	return func(tf typeFormatFunc) string {
		var b strings.Builder
		b.WriteString("field or method ")
		b.WriteString(ident)
		b.WriteString(" not found on ")
		b.WriteString(tf(tp))
		return b.String()
	}
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

// TreeFinder should wrap https://pkg.go.dev/html/template#Template.Lookup and return the Tree field from the Template
// If you are using text/template the lookup function from that package should also work.
type TreeFinder interface {
	FindTree(name string) (*template.Tree, bool)
}

type FindTreeFunc func(name string) (*template.Tree, bool)

func (fn FindTreeFunc) FindTree(name string) (*template.Tree, bool) {
	return fn(name)
}

// DefinitionFinder reports where a template name was defined. When a name
// was defined more than once it reports the definition that survived, the
// one template lookup resolves to.
type DefinitionFinder interface {
	FindDefinition(name string) (Definition, bool)
}

type FindDefinitionFunc func(name string) (Definition, bool)

func (fn FindDefinitionFunc) FindDefinition(name string) (Definition, bool) {
	return fn(name)
}

type CallChecker interface {
	CheckCall(*Global, string, []template.Node, []types.Type) (types.Type, error)
}

type TypeNodeMapping map[types.Type][]template.Node

// Execute type-checks tree against data, the type of the template's root
// context (dot). It returns nil when the template checks cleanly. Otherwise
// the returned error is a *Error: a single leaf for one failure, or an
// ErrorTypeAggregate node grouping every independent failure found. Walk the
// full tree with Error.All or Unwrap; each leaf carries the ErrorType
// classification, the template.Tree and template.Node it was found at, and,
// when relevant, the types.Type being checked.
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

	// failed records variables whose declaration pipeline failed to check,
	// so later lookups can be marked Secondary instead of reported as
	// independent failures.
	failed map[string]bool

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
		failed:      maps.Clone(s.failed),
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

// markFailedDecls records the variables a failing pipeline would have
// declared.
func (s *scope) markFailedDecls(n *template.PipeNode) {
	for _, decl := range n.Decl {
		if len(decl.Ident) == 0 {
			continue
		}
		if s.failed == nil {
			s.failed = make(map[string]bool)
		}
		s.failed[decl.Ident[0]] = true
	}
}

// walkEach walks nodes in order, collecting every failure rather than
// stopping at the first.
func (s *scope) walkEach(tree *template.Tree, dot, prev types.Type, node template.Node, nodes ...template.Node) error {
	var errs []error
	for _, child := range nodes {
		if _, err := s.walk(tree, dot, prev, child); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(tree, node, errs...)
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
		return nil, s.walkEach(tree, dot, prev, n, n.Children...)
	case *template.HTMLElementNode:
		return nil, s.walkEach(tree, dot, prev, n, append(slices.Clip(n.Attr), n.Children...)...)
	case *template.HTMLAttributeNode:
		return nil, s.walkEach(tree, dot, prev, n, append(slices.Clip(n.Key), n.Value...)...)
	case *template.HTMLCommentNode:
		return nil, s.walkEach(tree, dot, prev, n, n.Children...)
	case *template.HTMLDoctypeNode:
		return nil, nil
	case *template.DefineNode:
		return nil, nil
	case *template.BlockNode:
		return nil, s.checkBlockNode(tree, dot, prev, n)
	default:
		return nil, newError(ErrorTypeUnexpectedNode, tree, n, "missing node type check %T", n)
	}
}

func (s *scope) checkBlockNode(tree *template.Tree, dot, _ types.Type, n *template.BlockNode) error {
	x := dot
	if n.Pipe != nil {
		tp, err := s.walk(tree, x, nil, n.Pipe)
		if err != nil {
			// The body's dot is unknown when the pipe fails, so only the
			// pipe is checked.
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
		e := newError(ErrorTypeVariableNotFound, tree, n, "variable %s not found", n.Ident[0])
		e.Secondary = s.failed[n.Ident[0]]
		return nil, e
	}
	return s.checkIdentifiers(tree, tp, n, n.Ident[1:], args)
}

func (s *scope) checkListNode(tree *template.Tree, dot, prev types.Type, n *template.ListNode) error {
	return s.walkEach(tree, dot, prev, n, n.Nodes...)
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
			s.markFailedDecls(n)
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
	var errs []error
	_, pipeErr := s.walk(tree, dot, nil, n.Pipe)
	if pipeErr != nil {
		errs = append(errs, pipeErr)
	} else if truth, ok := literalCondition(n.Pipe); ok {
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
		errs = append(errs, err)
	}
	if n.ElseList != nil {
		elseScope := s.child()
		if _, err := elseScope.walk(tree, dot, nil, n.ElseList); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(tree, n, errs...)
}

func (s *scope) checkWithNode(tree *template.Tree, dot types.Type, n *template.WithNode) error {
	var errs []error
	child := s.child()
	x, err := child.walk(tree, dot, nil, n.Pipe)
	if err != nil {
		// The body's dot is unknown when the pipe fails, so only the
		// pipe and the else list (which keeps the outer dot) are checked.
		errs = append(errs, err)
	} else {
		withScope := child.child()
		// {{with}} enters its body only when the subject is non-empty, so
		// the body dot is guaranteed non-nil — dereferencing it is safe
		// (W003).
		if withScope.guarded == nil {
			withScope.guarded = make(map[string]bool)
		}
		withScope.guarded[""] = true
		if _, err := withScope.walk(tree, x, nil, n.List); err != nil {
			errs = append(errs, err)
		}
	}
	if n.ElseList != nil {
		elseScope := child.child()
		if _, err := elseScope.walk(tree, dot, nil, n.ElseList); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(tree, n, errs...)
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
			return nil, newError(ErrorTypeConstantOverflow, tree, constant, "%s overflows int", constant.Text)
		}
		return types.Typ[types.UntypedInt], nil

	case constant.IsUint:
		return nil, newError(ErrorTypeConstantOverflow, tree, constant, "%s overflows int", constant.Text)
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
	var errs []error
	pipeOK := true
	if n.Pipe != nil {
		tp, err := s.walk(tree, x, nil, n.Pipe)
		if err != nil {
			// The invoked template's dot is unknown when the pipe fails,
			// so only the pipe and the template lookup are checked.
			errs = append(errs, err)
			pipeOK = false
		} else {
			x = downgradeUntyped(tp)
		}
	} else {
		x = types.Typ[types.UntypedNil]
	}
	if fn := s.global.InspectTemplateNode; fn != nil && pipeOK {
		fn(n, tree, x, s.global.definition(n.Name))
	}
	childTree, ok := s.global.trees.FindTree(n.Name)
	if !ok {
		notFound := newError(ErrorTypeTemplateNotFound, tree, n, "template %q not found", n.Name)
		if pipeOK {
			notFound.X = x
		}
		errs = append(errs, notFound)
		return joinErrors(tree, n, errs...)
	}
	if pipeOK && !s.invoking(childTree, x) {
		childScope := newTemplateScope(s.global, childTree, x, s.invocations)
		if _, err := childScope.walk(childTree, x, nil, childTree.Root); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(tree, n, errs...)
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
			return nil, wrapError(ErrorTypeUnknown, tree, cmd, err)
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
		return nil, newError(ErrorTypeBadCommand, tree, n, "nil is not a command")
	default:
		return nil, newError(ErrorTypeBadCommand, tree, first, "can't evaluate command %q", first)
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
	var errs []error
	for _, arg := range args {
		argType, err := child.walk(tree, dot, prev, arg)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		argTypes = append(argTypes, argType)
		if path, ok := fieldPathOf(arg); ok {
			child.addGuard(path)
		}
	}
	if err := joinErrors(tree, nil, errs...); err != nil {
		return nil, err
	}
	if prev != nil {
		argTypes = append(argTypes, prev)
	}
	return argTypes, nil
}

func (s *scope) argumentTypes(tree *template.Tree, dot types.Type, prev types.Type, args []template.Node) ([]types.Type, error) {
	argTypes := make([]types.Type, 0, len(args)+1)
	var errs []error
	for _, arg := range args {
		argType, err := s.walk(tree, dot, prev, arg)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		argTypes = append(argTypes, argType)
	}
	if err := joinErrors(tree, nil, errs...); err != nil {
		return nil, err
	}
	if prev != nil {
		argTypes = append(argTypes, prev)
	}
	return argTypes, nil
}

func (s *scope) notAFunction(tree *template.Tree, node template.Node, args []template.Node, final types.Type) error {
	if len(args) > 1 || final != nil {
		return newError(ErrorTypeNotAFunction, tree, node, "can't give argument to non-function %s", args[0])
	}
	return nil
}

// identErr builds an *Error wrapping an *IdentifierError. The location
// prefix is added by *Error at format time; the IdentifierError carries
// the full type so callers of FormatVerbose can render its structure.
// types.Type args are rendered with a nil qualifier in the Cause message
// and re-rendered with the caller's qualifier by DetailedError.
func (s *scope) identErr(errType ErrorType, tree *template.Tree, n template.Node, ident string, tp types.Type, format string, a ...any) *Error {
	render := renderer(format, a...)
	return wrapError(errType, tree, n, &IdentifierError{
		Identifier: ident,
		Type:       tp,
		Cause:      errors.New(render(fullTypeFormat(nil))),
		render:     render,
		qualifier:  s.global.Qualifier,
		fset:       s.global.fileSet,
	}).withX(tp)
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
						return nil, s.identErr(ErrorTypeMapKey, tree, n, ident, xx, `can't evaluate field one in type %s`, xx)
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
				return nil, s.identErr(ErrorTypeFieldNotExported, tree, n, ident, x, "field or method %s is not exported", ident)
			}
			obj, _, _ := types.LookupFieldOrMethod(x, true, s.global.pkg, ident)
			if obj == nil {
				// Accessing a field on an interface value cannot be verified
				// statically. With Global.WarnInterfaceFieldAccess, warn (once
				// per chain) and continue as an unknown value, matching how the
				// template resolves it at runtime; otherwise report it as not
				// found, as upstream does.
				if types.IsInterface(x) && s.global.WarnInterfaceFieldAccess {
					if !warnedInterface {
						s.warn(tree, n, WarnInterfaceFieldAccess, "%s is accessed on interface value %s and cannot be checked until runtime", ident, s.global.TypeString(x))
						warnedInterface = true
					}
					x = emptyInterface
					continue
				}
				render := notFoundMessage(ident, x)
				notFound := wrapError(ErrorTypeFieldOrMethodNotFound, tree, n, &IdentifierError{
					Identifier: ident,
					Type:       x,
					Cause:      errors.New(render(fullTypeFormat(nil))),
					render:     render,
					qualifier:  s.global.Qualifier,
					fset:       s.global.fileSet,
				}).withX(x)
				if named, ok := x.(*types.Named); ok && s.global.fileSet != nil {
					notFound.Decl = s.global.fileSet.Position(named.Obj().Pos())
				}
				return nil, notFound
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
					return nil, s.identErr(ErrorTypeBadSignature, tree, n, ident, sig, "function %s has %d return values; should be 1 or 2", ident, resultLen).withDecl(methodPos)
				}
				if resultLen > 1 {
					methodPos := s.global.fileSet.Position(obj.Pos())
					finalResult := sig.Results().At(sig.Results().Len() - 1)
					errorType := types.Universe.Lookup("error")
					if !types.Identical(errorType.Type(), finalResult.Type()) {
						return nil, s.identErr(ErrorTypeBadSignature, tree, n, ident, sig, "invalid function signature for %s: second return value should be error; is %s", ident, finalResult.Type()).withDecl(methodPos)
					}
				}
				if i == len(idents)-1 {
					res, err := checkCallArguments(s.global, ident, sig, args)
					if err != nil {
						return nil, wrapError(ErrorTypeCallArguments, tree, n, err)
					}
					return res, nil
				}
				// Before the end of a chain, text/template calls a
				// method with no arguments.
				if _, err := checkCallArguments(s.global, ident, sig, nil); err != nil {
					return nil, wrapError(ErrorTypeCallArguments, tree, n, err)
				}
				x = sig.Results().At(0).Type()
			}
			if _, ok := x.(*types.Signature); ok && i < len(idents)-1 {
				return nil, s.identErr(ErrorTypeIdentifierChain, tree, n, ident, x, "identifier chain not supported for type %s", x)
			}
		}
	}
	if len(args) > 0 {
		sig, ok := x.(*types.Signature)
		if !ok {
			return nil, s.identErr(ErrorTypeNotAFunction, tree, n, "", x, "expected method or function")
		}
		var name string
		if len(idents) > 0 {
			name = idents[len(idents)-1]
		}
		tp, err := checkCallArguments(s.global, name, sig, args)
		if err != nil {
			return nil, wrapError(ErrorTypeCallArguments, tree, n, err)
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
			return newError(ErrorTypeRange, tree, n.Pipe, "range can't iterate over %s", untypedTrimmed{pipeType}).withX(pipeType)
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
				return newError(ErrorTypeRange, tree, n.Pipe, "iter.Seq[T] must not iterate over more than one variable").withX(pipeType)
			}
		} else {
			return newError(ErrorTypeRange, tree, n.Pipe, "failed to range over function %s", pipeType).withX(pipeType)
		}
	default:
		return newError(ErrorTypeRange, tree, n.Pipe, "failed to range over %s", pipeType).withX(pipeType)
	}
	// A range element — the body dot and the loop variables — may be a
	// pointer, but flagging every {{range .Ptrs}}{{.Field}} would be noise:
	// the common case has no nil elements. Treat the element as guarded for
	// the W003 nil-pointer warning.
	if child.guarded == nil {
		child.guarded = make(map[string]bool)
	}
	child.guarded[""] = true
	var errs []error
	if _, err := child.walk(tree, x, nil, n.List); err != nil {
		errs = append(errs, err)
	}
	if n.ElseList != nil {
		if _, err := child.walk(tree, x, nil, n.ElseList); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(tree, n, errs...)
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
			return nil, wrapError(ErrorTypeUnknown, tree, n, err)
		}
		return tp, err
	}
	tp, ok := s.variables[n.Ident]
	if !ok {
		e := newError(ErrorTypeVariableNotFound, tree, n, "failed to find identifier %s", n.Ident)
		e.Secondary = s.failed[n.Ident]
		return nil, e
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
