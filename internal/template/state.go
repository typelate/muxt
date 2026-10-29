package template

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type variable struct {
	name  string
	value reflect.Value
}

// state tracks the execution state of a single template.
type state struct {
	wr         io.Writer
	tree       *Tree
	cp         *CoverProfile
	vars       []variable
	depth      int
	funcs      FuncMap
	finder     TreeFinder
	missingKey MissingKeyMode
	trust      TrustFunc
}

// trustOf reports the Trust classification for a value, returning
// TrustNone when no TrustFunc was provided.
func (s *state) trustOf(v reflect.Value) Trust {
	if s.trust == nil {
		return TrustNone
	}
	return s.trust(v)
}

func (s *state) execute(dot reflect.Value) error {
	s.cp.Init(s.tree)
	root := s.tree.Root
	if root == nil {
		return nil
	}
	// Check if the root's first child is an HTMLDocumentFragmentNode.
	if len(root.Nodes) == 1 {
		if frag, ok := root.Nodes[0].(*HTMLDocumentFragmentNode); ok {
			return s.executeHTML(dot, frag)
		}
	}
	// Check if the root contains HTML nodes (e.g., from extracted {{define}}
	// blocks in HTML templates), or the tree was parsed in HTML mode. Both
	// require the HTML execution path so contextual escaping applies.
	if s.tree.HTMLMode || rootHasHTMLNodes(root) {
		return s.executeHTMLList(dot, root)
	}
	return s.walk(dot, root)
}

func (s *state) executeHTML(dot reflect.Value, frag *HTMLDocumentFragmentNode) error {
	doc, err := s.buildDocument(dot, frag)
	if err != nil {
		return err
	}
	return render(s.wr, doc)
}

// executeHTMLList handles a ListNode that contains HTML nodes directly
// (without an HTMLDocumentFragmentNode wrapper). This occurs for trees
// extracted from {{define}} or {{block}} in HTML-mode templates.
// RootTrimFirst/RootTrimLast (set by extract.go and blockControl) carry
// the defining {{define ... -}}/{{block ... -}} and {{- end}} node's own
// trim markers, which would otherwise be lost: the body has no siblings
// once it is its own tree, so nothing else could apply them.
func (s *state) executeHTMLList(dot reflect.Value, list *ListNode) error {
	nodes, err := s.walkHTMLList(dot, list, s.tree.RootTrimFirst, s.tree.RootTrimLast)
	if err != nil {
		return err
	}
	doc := &html.Node{Type: html.DocumentNode}
	for _, n := range nodes {
		doc.AppendChild(n)
	}
	return render(s.wr, doc)
}

// rootHasHTMLNodes reports whether a ListNode contains any HTML-specific
// node types (HTMLElementNode, HTMLCommentNode, HTMLDoctypeNode).
func rootHasHTMLNodes(list *ListNode) bool {
	for _, n := range list.Nodes {
		switch n.(type) {
		case *HTMLElementNode, *HTMLCommentNode, *HTMLDoctypeNode:
			return true
		}
	}
	return false
}

// buildDocument builds an *html.Node (DocumentNode) from an HTML fragment.
func (s *state) buildDocument(dot reflect.Value, frag *HTMLDocumentFragmentNode) (*html.Node, error) {
	doc := &html.Node{
		Type: html.DocumentNode,
	}
	for _, child := range frag.Children {
		nodes, err := s.walkHTMLNode(dot, child)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			doc.AppendChild(n)
		}
	}
	return doc, nil
}

func (s *state) walk(dot reflect.Value, node Node) error {
	switch n := node.(type) {
	case *ListNode:
		return s.walkList(dot, n, false, false)
	case *TextNode:
		n.count.Add(1)
		if _, err := s.wr.Write(n.Text); err != nil {
			return s.writeError(err)
		}
	case *ActionNode:
		return s.walkAction(dot, n)
	case *IfNode:
		return s.walkIfNode(dot, n)
	case *RangeNode:
		return s.walkRangeNode(dot, n)
	case *WithNode:
		return s.walkWithNode(dot, n)
	case *TemplateNode:
		return s.walkTemplateNode(dot, n)
	case *BreakNode:
		n.count.Add(1)
		return breakError{}
	case *ContinueNode:
		n.count.Add(1)
		return continueError{}
	case *CommentNode:
		// ExecuteTemplate comments produce no output.
	case *DefineNode:
		// Define nodes are processed at parse time, not execution time.
	case *BlockNode:
		// Block nodes act like {{template "name" pipeline}} at execution time.
		// Extract() should have registered the block body as a named template.
		return s.walkTemplateNode(dot, s.tree.newTemplate(n.Start(), n.End(), n.Line, n.Name, n.Pipe))
	default:
		return s.errorf("unknown node: %s", node)
	}
	return nil
}

func (s *state) walkList(dot reflect.Value, list *ListNode, trimFirst, trimLast bool) error {
	nodes := list.Nodes
	for i, node := range nodes {
		if tn, ok := node.(*TextNode); ok {
			text := tn.Text
			if (i == 0 && trimFirst) || (i > 0 && nodeRightTrim(nodes[i-1])) {
				text = bytes.TrimLeft(text, trimCutset)
			}
			if (i == len(nodes)-1 && trimLast) || (i < len(nodes)-1 && nodeLeftTrim(nodes[i+1])) {
				text = bytes.TrimRight(text, trimCutset)
			}
			tn.count.Add(1)
			if len(text) > 0 {
				if _, err := s.wr.Write(text); err != nil {
					return s.writeError(err)
				}
			}
			continue
		}
		if err := s.walk(dot, node); err != nil {
			return err
		}
	}
	return nil
}

const trimCutset = " \t\r\n"

// nodeLeftTrim reports whether a node trims whitespace from preceding text.
func nodeLeftTrim(n Node) bool {
	switch n := n.(type) {
	case *ActionNode:
		return n.TrimLeft
	case *IfNode:
		return n.StartTrimLeft
	case *RangeNode:
		return n.StartTrimLeft
	case *WithNode:
		return n.StartTrimLeft
	case *TemplateNode:
		return n.TrimLeft
	case *CommentNode:
		return n.TrimLeft
	case *BreakNode:
		return n.TrimLeft
	case *ContinueNode:
		return n.TrimLeft
	case *DefineNode:
		return n.StartTrimLeft
	}
	return false
}

// nodeRightTrim reports whether a node trims whitespace from following text.
func nodeRightTrim(n Node) bool {
	switch n := n.(type) {
	case *ActionNode:
		return n.TrimRight
	case *IfNode:
		return n.EndTrimRight
	case *RangeNode:
		return n.EndTrimRight
	case *WithNode:
		return n.EndTrimRight
	case *TemplateNode:
		return n.TrimRight
	case *CommentNode:
		return n.TrimRight
	case *BreakNode:
		return n.TrimRight
	case *ContinueNode:
		return n.TrimRight
	case *DefineNode:
		return n.EndTrimRight
	}
	return false
}

func (s *state) walkAction(dot reflect.Value, action *ActionNode) error {
	action.count.Add(1)
	val, err := s.evalPipeline(dot, action.Pipe)
	if err != nil {
		return err
	}
	if len(action.Pipe.Decl) > 0 {
		// Variable assignment/declaration — no output.
		return nil
	}
	if _, err := s.printValue(s.wr, val); err != nil {
		return s.writeError(err)
	}
	return nil
}

func (s *state) walkIfNode(dot reflect.Value, n *IfNode) error {
	return s.walkBranch(dot, &n.BranchNode, "if")
}

func (s *state) walkWithNode(dot reflect.Value, n *WithNode) error {
	return s.walkBranch(dot, &n.BranchNode, "with")
}

func (s *state) walkBranch(dot reflect.Value, branch *BranchNode, name string) error {
	branch.openCount.Add(1)
	pipe, err := s.evalPipeline(dot, branch.Pipe)
	if err != nil {
		return err
	}
	truth, ok := isTrue(indirectInterface(pipe))
	if !ok {
		return s.errorf("if/with can't use %v", pipe)
	}
	var walkErr error
	if truth {
		newDot := dot
		if name == "with" {
			newDot = pipe
		}
		lastTrim := branch.EndTrimLeft
		if branch.ElseList != nil && len(branch.ElseList.Nodes) > 0 {
			lastTrim = branch.ElseTrimLeft
		}
		walkErr = s.walkList(newDot, branch.List, branch.StartTrimRight, lastTrim)
	} else if branch.ElseList != nil {
		branch.elseCount.Add(1)
		walkErr = s.walkList(dot, branch.ElseList, branch.ElseTrimRight, branch.EndTrimLeft)
	}
	if walkErr != nil {
		return walkErr
	}
	branch.endCount.Add(1)
	return nil
}

// walkRangeNode handles {{range}} iteration.
func (s *state) walkRangeNode(dot reflect.Value, r *RangeNode) error {
	r.BranchNode.openCount.Add(1)
	val, err := s.evalPipeline(dot, r.Pipe)
	if err != nil {
		return err
	}
	val, _ = indirect(val)
	mark := len(s.vars)
	oneIteration := func(index, elem reflect.Value) error {
		if len(r.Pipe.Decl) > 0 {
			if r.Pipe.IsAssign {
				// Assign mode: update existing variables
				if len(r.Pipe.Decl) > 1 {
					s.setVar(r.Pipe.Decl[0].Ident[0], index)
				}
				s.setVar(r.Pipe.Decl[len(r.Pipe.Decl)-1].Ident[0], elem)
			} else {
				// Declaration: set top of stack
				s.setTopVar(1, elem)
				if len(r.Pipe.Decl) > 1 {
					s.setTopVar(2, index)
				}
			}
		}
		lastTrim := r.EndTrimLeft
		if r.ElseList != nil && len(r.ElseList.Nodes) > 0 {
			lastTrim = r.ElseTrimLeft
		}
		return s.walkList(elem, r.List, r.StartTrimRight, lastTrim)
	}
	switch val.Kind() {
	case reflect.Array, reflect.Slice:
		if val.Len() == 0 {
			break
		}
		for i := 0; i < val.Len(); i++ {
			if err := oneIteration(reflect.ValueOf(i), val.Index(i)); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					continue
				}
				return err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return nil
	case reflect.Map:
		if val.Len() == 0 {
			break
		}
		iter := val.MapRange()
		for iter.Next() {
			if err := oneIteration(iter.Key(), iter.Value()); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					continue
				}
				return err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return nil
	case reflect.Chan:
		if val.IsNil() {
			break
		}
		i := 0
		for {
			elem, ok := val.Recv()
			if !ok {
				break
			}
			if err := oneIteration(reflect.ValueOf(i), elem); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					i++
					continue
				}
				return err
			}
			i++
		}
		if i == 0 {
			break
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		count := val.Int()
		if count <= 0 {
			break
		}
		for i := int64(0); i < count; i++ {
			if err := oneIteration(reflect.ValueOf(int(i)), reflect.ValueOf(int(i))); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					continue
				}
				return err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		count := val.Uint()
		if count == 0 {
			break
		}
		for i := uint64(0); i < count; i++ {
			if err := oneIteration(reflect.ValueOf(int(i)), reflect.ValueOf(int(i))); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					continue
				}
				return err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return nil
	case reflect.Invalid:
		// nil is like empty
		break
	default:
		return s.errorf("range can't iterate over %v", val)
	}
	// Empty — run else if present.
	s.vars = s.vars[:mark]
	if r.ElseList != nil {
		r.BranchNode.elseCount.Add(1)
		walkErr := s.walkList(dot, r.ElseList, r.ElseTrimRight, r.EndTrimLeft)
		if walkErr != nil {
			return walkErr
		}
		r.BranchNode.endCount.Add(1)
		return nil
	}
	r.BranchNode.endCount.Add(1)
	return nil
}

func (s *state) walkTemplateNode(dot reflect.Value, t *TemplateNode) error {
	t.count.Add(1)
	if s.depth >= maxExecDepth {
		return s.errorf("exceeded maximum template depth (%d)", maxExecDepth)
	}
	if s.finder == nil {
		return s.errorf("template %q not defined", t.Name)
	}
	tree, ok := s.finder.FindTree(t.Name)
	if !ok {
		return s.errorf("template %q not defined", t.Name)
	}
	newDot := dot
	if t.Pipe != nil {
		var err error
		newDot, err = s.evalPipeline(dot, t.Pipe)
		if err != nil {
			return err
		}
	}
	newState := &state{
		wr:     s.wr,
		tree:   tree,
		cp:     s.cp,
		depth:  s.depth + 1,
		funcs:  s.funcs,
		finder: s.finder,
		trust:  s.trust,
		vars:   []variable{{"$", newDot}},
	}
	return newState.execute(newDot)
}

// evalPipeline evaluates a pipeline and returns its final value.
func (s *state) evalPipeline(dot reflect.Value, pipe *PipeNode) (reflect.Value, error) {
	if pipe == nil {
		return missingVal, nil
	}
	var value reflect.Value
	for _, cmd := range pipe.Commands {
		var err error
		value, err = s.evalCommand(dot, cmd, value)
		if err != nil {
			return missingVal, err
		}
	}
	for _, vr := range pipe.Decl {
		if pipe.IsAssign {
			s.setVar(vr.Ident[0], value)
		} else {
			s.push(vr.Ident[0], value)
		}
	}
	return value, nil
}

// evalCommand evaluates a single command in a pipeline. The final argument is the
// result of the previous command (for pipe chaining), or invalid for the first command.
func (s *state) evalCommand(dot reflect.Value, cmd *CommandNode, final reflect.Value) (reflect.Value, error) {
	firstWord := cmd.Args[0]
	switch n := firstWord.(type) {
	case *FieldNode:
		return s.evalFieldNode(dot, n, cmd.Args, final)
	case *ChainNode:
		return s.evalChainNode(dot, n, cmd.Args, final)
	case *IdentifierNode:
		return s.evalFunction(dot, n, cmd, cmd.Args, final)
	case *PipeNode:
		return s.evalPipeline(dot, n)
	case *VariableNode:
		return s.evalVariableNode(dot, n, cmd.Args, final)
	}
	notAFunction := len(cmd.Args) > 1 || final.IsValid()
	if notAFunction {
		return missingVal, s.errorf("can't give argument to non-function %s", firstWord)
	}
	switch n := firstWord.(type) {
	case *BoolNode:
		return reflect.ValueOf(n.True), nil
	case *DotNode:
		return dot, nil
	case *NilNode:
		return reflect.ValueOf(nil), nil
	case *NumberNode:
		return s.idealConstant(n)
	case *StringNode:
		return reflect.ValueOf(n.Text), nil
	}
	return missingVal, s.errorf("can't evaluate command %q", firstWord)
}

func (s *state) idealConstant(constant *NumberNode) (reflect.Value, error) {
	switch {
	case constant.IsComplex:
		return reflect.ValueOf(constant.Complex128), nil
	case constant.IsFloat &&
		!isHexInt(constant.Text) && !isRuneInt(constant.Text) &&
		strings.ContainsAny(constant.Text, ".eEpP"):
		return reflect.ValueOf(constant.Float64), nil
	case constant.IsInt:
		n := int(constant.Int64)
		if int64(n) != constant.Int64 {
			return missingVal, s.errorf("%s overflows int", constant.Text)
		}
		return reflect.ValueOf(n), nil
	case constant.IsUint:
		return missingVal, s.errorf("%s overflows int", constant.Text)
	}
	return missingVal, nil
}

func isHexInt(s string) bool {
	return len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') && !strings.ContainsAny(s, "pP")
}

func isRuneInt(s string) bool {
	return len(s) > 0 && s[0] == '\''
}

func (s *state) evalFieldNode(dot reflect.Value, field *FieldNode, args []Node, final reflect.Value) (reflect.Value, error) {
	return s.evalFieldChain(dot, dot, field.Ident, args, final)
}

func (s *state) evalChainNode(dot reflect.Value, chain *ChainNode, args []Node, final reflect.Value) (reflect.Value, error) {
	if len(chain.Field) == 0 {
		return missingVal, s.errorf("internal error: no fields in evalChainNode")
	}
	pipe, err := s.evalArg(dot, nil, chain.Node)
	if err != nil {
		return missingVal, err
	}
	return s.evalFieldChain(dot, pipe, chain.Field, args, final)
}

func (s *state) evalVariableNode(dot reflect.Value, v *VariableNode, args []Node, final reflect.Value) (reflect.Value, error) {
	value := s.varValue(v.Ident[0])
	if len(v.Ident) == 1 {
		notAFunction := len(args) > 1 || final.IsValid()
		if notAFunction {
			return missingVal, s.errorf("can't give argument to non-function %s", v.Ident[0])
		}
		return value, nil
	}
	return s.evalFieldChain(dot, value, v.Ident[1:], args, final)
}

func (s *state) evalFieldChain(dot, receiver reflect.Value, ident []string, args []Node, final reflect.Value) (reflect.Value, error) {
	n := len(ident)
	for i := 0; i < n-1; i++ {
		var err error
		// A non-final field is always a zero-argument field or method
		// access. evalCall treats args[0] as the implicit word and counts
		// len(args)-1 fixed arguments, so a zero-arg access must pass the
		// one-element sentinel (not nil, which would count as -1 and reject
		// a genuine zero-parameter method such as .Result.Name).
		receiver, err = s.evalField(dot, ident[i], zeroArgSentinel, missingVal, receiver)
		if err != nil {
			return missingVal, err
		}
	}
	return s.evalField(dot, ident[n-1], args, final, receiver)
}

// zeroArgSentinel is a one-element argument slice representing a
// zero-argument field or method access under evalCall's args[0]-is-the-word
// convention. Its sole element is never dereferenced.
var zeroArgSentinel = []Node{nil}

func (s *state) evalFunction(dot reflect.Value, node *IdentifierNode, cmd *CommandNode, args []Node, final reflect.Value) (reflect.Value, error) {
	name := node.Ident
	// The built-in and/or short-circuit: they evaluate arguments left to
	// right and stop at the first that decides the result (an empty argument
	// for and, a non-empty one for or), matching text/template since Go 1.18.
	// A user-supplied func of the same name is a normal call, not short-
	// circuited; and a zero-argument and/or falls through to report the
	// usual arg-count error.
	if name == "and" || name == "or" {
		if _, overridden := s.funcs[name]; !overridden && (len(args) > 1 || final.IsValid()) {
			return s.evalAndOr(dot, name, args, final)
		}
	}
	fn, ok := s.findFunction(name)
	if !ok {
		return missingVal, s.errorf("function %q not defined", name)
	}
	return s.evalCall(dot, fn, cmd, name, args, final)
}

// evalAndOr evaluates a built-in and/or over its operands (args[1:] then the
// piped final value) left to right, short-circuiting. It returns the operand
// that decides the result — the first empty one for and, the first non-empty
// one for or — or the last operand when none decides, without evaluating any
// operand past the decider.
func (s *state) evalAndOr(dot reflect.Value, name string, args []Node, final reflect.Value) (reflect.Value, error) {
	returnOnTruth := name == "or"
	var result reflect.Value
	decided := func(v reflect.Value) bool {
		result = v
		return truth(v) == returnOnTruth
	}
	for i := 1; i < len(args); i++ {
		v, err := s.evalArg(dot, nil, args[i])
		if err != nil {
			return missingVal, err
		}
		if decided(v) {
			return result, nil
		}
	}
	if final.IsValid() && decided(final) {
		return result, nil
	}
	return result, nil
}

func (s *state) findFunction(name string) (reflect.Value, bool) {
	if s.funcs != nil {
		if fn, ok := s.funcs[name]; ok {
			return reflect.ValueOf(fn), true
		}
	}
	if fn, ok := builtins()[name]; ok {
		return reflect.ValueOf(fn), true
	}
	return missingVal, false
}

func (s *state) evalField(dot reflect.Value, fieldName string, args []Node, final reflect.Value, receiver reflect.Value) (reflect.Value, error) {
	if !receiver.IsValid() {
		return missingVal, s.errorf("nil pointer evaluating %s.%s", "<nil>", fieldName)
	}
	typ := receiver.Type()
	receiver = indirectInterface(receiver)
	hasArgs := len(args) > 1 || final.IsValid()

	// Try methods first (on the pointer receiver too).
	ptr := receiver
	if ptr.Kind() != reflect.Interface && ptr.CanAddr() {
		ptr = ptr.Addr()
	}
	if method := ptr.MethodByName(fieldName); method.IsValid() {
		return s.evalCall(dot, method, nil, fieldName, args, final)
	}
	// Try methods on the value itself.
	if method := receiver.MethodByName(fieldName); method.IsValid() {
		return s.evalCall(dot, method, nil, fieldName, args, final)
	}
	if hasArgs {
		return missingVal, s.errorf("can't call method/function %q on %s", fieldName, typ)
	}
	// Try struct fields, dereferencing a pointer receiver first (methods
	// were already tried above on both the pointer and the value; a bare
	// struct field is only reachable through the pointer's Elem, e.g.
	// *http.Request.Pattern).
	structReceiver, isNil := indirect(receiver)
	if isNil {
		return missingVal, s.errorf("nil pointer evaluating field %s in type %s", fieldName, typ)
	}
	switch structReceiver.Kind() {
	case reflect.Struct:
		field := structReceiver.FieldByName(fieldName)
		if field.IsValid() {
			return field, nil
		}
	case reflect.Map:
		nameVal := reflect.ValueOf(fieldName)
		if nameVal.Type().AssignableTo(structReceiver.Type().Key()) {
			result := structReceiver.MapIndex(nameVal)
			if result.IsValid() {
				return result, nil
			}
			switch s.missingKey {
			case MissingKeyError:
				return missingVal, s.errorf("map has no entry for key %q", fieldName)
			case MissingKeyZero:
				return reflect.Zero(structReceiver.Type().Elem()), nil
			default: // MissingKeyInvalid
				return reflect.Value{}, nil
			}
		}
	default:
	}
	return missingVal, s.errorf("can't evaluate field %s in type %s", fieldName, typ)
}

func (s *state) evalCall(dot reflect.Value, fun reflect.Value, cmd *CommandNode, name string, args []Node, final reflect.Value) (reflect.Value, error) {
	typ := fun.Type()
	numIn := typ.NumIn()
	numFixed := len(args) - 1 // first arg is the function name itself
	if final.IsValid() {
		numFixed++
	}
	// For non-variadic, numFixed must equal numIn.
	// For variadic, numFixed must be >= numIn-1.
	if !typ.IsVariadic() {
		if numFixed != numIn {
			return missingVal, s.errorf("wrong number of args for %s: want %d got %d", name, numIn, numFixed)
		}
	} else {
		if numFixed < numIn-1 {
			return missingVal, s.errorf("wrong number of args for %s: want at least %d got %d", name, numIn-1, numFixed)
		}
	}
	if !goodFunc(typ) {
		return missingVal, s.errorf("can't call %s: invalid return signature", name)
	}

	// Build the argument list.
	argv := make([]reflect.Value, 0, numFixed)
	// Evaluate the explicit arguments (skipping the first arg which is the function reference).
	for i := 1; i < len(args); i++ {
		var paramType reflect.Type
		if !typ.IsVariadic() || i-1 < numIn-1 {
			paramType = typ.In(i - 1)
		} else {
			paramType = typ.In(numIn - 1).Elem()
		}
		arg, err := s.evalArg(dot, paramType, args[i])
		if err != nil {
			return missingVal, err
		}
		argv = append(argv, s.validateArg(paramType, arg))
	}
	// Add the piped (final) argument.
	if final.IsValid() {
		var paramType reflect.Type
		idx := len(argv)
		if !typ.IsVariadic() || idx < numIn-1 {
			if idx < numIn {
				paramType = typ.In(idx)
			}
		} else {
			paramType = typ.In(numIn - 1).Elem()
		}
		argv = append(argv, s.validateArg(paramType, final))
	}

	result, err := safeCall(fun, argv)
	if err != nil {
		return missingVal, s.errorf("error calling %s: %w", name, err)
	}
	return result, nil
}

func (s *state) evalArg(dot reflect.Value, typ reflect.Type, n Node) (reflect.Value, error) {
	switch arg := n.(type) {
	case *DotNode:
		return dot, nil
	case *NilNode:
		if typ != nil && canBeNil(typ) {
			return reflect.Zero(typ), nil
		}
		return reflect.ValueOf(nil), nil
	case *FieldNode:
		// zeroArgSentinel, not nil: a bare field/method reference used as
		// one argument among several (e.g. .Result in {{printf "%v"
		// .Result}}) still reaches evalCall for its final segment, which
		// computes fixed-arg count as len(args)-1. nil would compute -1 and
		// reject a genuine zero-parameter method the same way an
		// unguarded non-final chain segment once did (see evalFieldChain).
		return s.evalFieldNode(dot, arg, zeroArgSentinel, missingVal)
	case *VariableNode:
		return s.evalVariableNode(dot, arg, zeroArgSentinel, missingVal)
	case *PipeNode:
		return s.evalPipeline(dot, arg)
	case *IdentifierNode:
		return s.evalFunction(dot, arg, nil, []Node{arg}, missingVal)
	case *ChainNode:
		return s.evalChainNode(dot, arg, zeroArgSentinel, missingVal)
	case *BoolNode:
		return reflect.ValueOf(arg.True), nil
	case *NumberNode:
		return s.idealConstant(arg)
	case *StringNode:
		return reflect.ValueOf(arg.Text), nil
	}
	return missingVal, s.errorf("can't handle %s for arg", n)
}

// printValue writes the textual representation of the value to the writer.
func (s *state) printValue(wr io.Writer, v reflect.Value) (int, error) {
	if !v.IsValid() {
		return wr.Write([]byte("<no value>"))
	}
	iface := printableValue(v)
	if iface == nil {
		return 0, s.errorf("can't print %s of type %s", v, v.Type())
	}
	text := fmt.Sprint(iface)
	return wr.Write([]byte(text))
}

// printableValue returns the value fmt.Sprint should receive to print v:
// v itself for anything that isn't a pointer, or, for a pointer, the
// pointer's Elem when neither implements error or fmt.Stringer, so a nil
// pointer prints as "<nil>" instead of panicking a String/Error method
// on a nil receiver — the same tradeoff text/template makes.
//
// A pointer whose element type implements error/Stringer only through a
// pointer receiver (the common case: TemplateData's generated String
// method) must NOT be dereferenced — doing so would drop to the value
// type, which does not implement the interface, and fall through to a
// raw %v of every field. So the interface check runs on v before any
// dereferencing, not after.
func printableValue(v reflect.Value) any {
	if v.Type().Implements(errorType) || v.Type().Implements(stringerType) {
		if v.CanInterface() {
			return v.Interface()
		}
	}
	if v.Kind() == reflect.Pointer {
		v, _ = indirect(v)
		if !v.IsValid() {
			return "<nil>"
		}
	}
	if v.CanInterface() {
		return v.Interface()
	}
	return nil
}

var stringerType = reflect.TypeFor[fmt.Stringer]()

// Variable management

func (s *state) push(name string, value reflect.Value) {
	s.vars = append(s.vars, variable{name, value})
}

func (s *state) setVar(name string, value reflect.Value) {
	for i := len(s.vars) - 1; i >= 0; i-- {
		if s.vars[i].name == name {
			s.vars[i].value = value
			return
		}
	}
	s.vars = append(s.vars, variable{name, value})
}

func (s *state) setTopVar(n int, value reflect.Value) {
	s.vars[len(s.vars)-n].value = value
}

func (s *state) varValue(name string) reflect.Value {
	for i := len(s.vars) - 1; i >= 0; i-- {
		if s.vars[i].name == name {
			return s.vars[i].value
		}
	}
	return missingVal
}

// HTML walk methods

// walkHTMLNode dispatches to the appropriate HTML walk method.
// Returns a slice of html.Node to append to a parent.
func (s *state) walkHTMLNode(dot reflect.Value, node Node) ([]*html.Node, error) {
	switch n := node.(type) {
	case *HTMLElementNode:
		el, err := s.walkHTMLElement(dot, n)
		if err != nil {
			if el != nil {
				return []*html.Node{el}, err
			}
			return nil, err
		}
		return []*html.Node{el}, nil
	case *HTMLCommentNode:
		// Comments are stripped to match html/template behavior.
		n.count.Add(1)
		return nil, nil
	case *HTMLDoctypeNode:
		n.count.Add(1)
		dn := &html.Node{
			Type: html.DoctypeNode,
			Data: "html",
		}
		return []*html.Node{dn}, nil
	case *TextNode:
		n.count.Add(1)
		nodeType := html.TextNode
		if n.Verbatim {
			nodeType = html.RawNode
		}
		tn := &html.Node{
			Type: nodeType,
			Data: string(n.Text),
		}
		return []*html.Node{tn}, nil
	case *ActionNode:
		return s.walkHTMLAction(dot, n)
	case *IfNode:
		return s.walkHTMLIf(dot, n)
	case *RangeNode:
		return s.walkHTMLRange(dot, n)
	case *WithNode:
		return s.walkHTMLWith(dot, n)
	case *TemplateNode:
		return s.walkHTMLTemplate(dot, n)
	case *DefineNode:
		// Skip define nodes during execution.
		return nil, nil
	case *BlockNode:
		// Block nodes act like {{template "name" pipeline}} at execution time.
		return s.walkHTMLTemplate(dot, s.tree.newTemplate(n.Start(), n.End(), n.Line, n.Name, n.Pipe))
	case *BreakNode:
		n.count.Add(1)
		return nil, breakError{}
	case *ContinueNode:
		n.count.Add(1)
		return nil, continueError{}
	case *CommentNode:
		// ExecuteTemplate comments produce no output.
		return nil, nil
	default:
		return nil, s.errorf("unsupported HTML node type: %T", node)
	}
}

func (s *state) walkHTMLElement(dot reflect.Value, elem *HTMLElementNode) (*html.Node, error) {
	elem.openCount.Add(1)
	n := &html.Node{
		Type:     html.ElementNode,
		Data:     elem.TagName,
		DataAtom: htmlAtom(elem.TagName),
	}
	// Process attributes.
	for _, attrNode := range elem.Attr {
		switch a := attrNode.(type) {
		case *HTMLAttributeNode:
			a.count.Add(1)
			attr, err := s.evalHTMLAttribute(dot, a)
			if err != nil {
				return nil, err
			}
			n.Attr = append(n.Attr, attr)
		case *ActionNode:
			// Action in attribute position — evaluate and treat result as attribute text.
			val, err := s.evalPipeline(dot, a.Pipe)
			if err != nil {
				return nil, err
			}
			if !val.IsValid() {
				continue
			}
			iface := printableValue(val)
			if iface == nil {
				return nil, s.errorf("can't print %s of type %s", val, val.Type())
			}
			text := fmt.Sprint(iface)
			if text != "" {
				n.Attr = append(n.Attr, html.Attribute{Val: text})
			}
		case *IfNode:
			attrs, err := s.walkHTMLAttrBranch(dot, &a.BranchNode, "if")
			if err != nil {
				return nil, err
			}
			n.Attr = append(n.Attr, attrs...)
		case *RangeNode:
			attrs, err := s.walkHTMLAttrRange(dot, a)
			if err != nil {
				return nil, err
			}
			n.Attr = append(n.Attr, attrs...)
		case *WithNode:
			attrs, err := s.walkHTMLAttrBranch(dot, &a.BranchNode, "with")
			if err != nil {
				return nil, err
			}
			n.Attr = append(n.Attr, attrs...)
		default:
			return nil, s.errorf("unsupported attribute node type: %T", attrNode)
		}
	}
	elem.attrCount.Add(1)
	// Process children.
	for _, child := range elem.Children {
		nodes, err := s.walkHTMLNode(dot, child)
		for _, cn := range nodes {
			n.AppendChild(cn)
		}
		if err != nil {
			if isBreak(err) || isContinue(err) {
				// Preserve the partially-built element. The caller
				// (walkHTMLList) collects nodes before propagating.
				return n, err
			}
			return nil, err
		}
	}
	elem.closeCount.Add(1)
	return n, nil
}

func (s *state) evalHTMLAttribute(dot reflect.Value, attr *HTMLAttributeNode) (html.Attribute, error) {
	ns := attr.Namespace

	// Reject dynamic attribute names. Allowing actions in attribute-name
	// position lets a template's data choose what attribute is emitted —
	// for example, picking "onclick" — which silently bypasses the URL
	// and event-handler escapers, since their dispatch is keyed on the
	// statically known attribute name.
	for _, k := range attr.Key {
		if _, ok := k.(*TextNode); !ok {
			return html.Attribute{}, s.errorf("dynamic attribute names are not permitted (template action found in attribute name)")
		}
	}

	// Evaluate key.
	var keyBuf bytes.Buffer
	for _, k := range attr.Key {
		if tn, ok := k.(*TextNode); ok {
			keyBuf.Write(tn.Text)
		}
	}

	key := keyBuf.String()
	if err := validateAttrName(key); err != nil {
		return html.Attribute{}, s.errorf("invalid attribute name %q: %v", key, err)
	}

	if !attr.HasValue {
		return html.Attribute{Namespace: ns, Key: key}, nil
	}

	// Evaluate value, stripping surrounding quotes from the overall value.
	var valParts []string
	if err := s.evalHTMLAttrValueNodes(dot, attr.Value, &valParts); err != nil {
		return html.Attribute{}, err
	}
	valStr := strings.Join(valParts, "")
	valStr = stripAttrQuotes(valStr)

	return html.Attribute{Namespace: ns, Key: key, Val: valStr}, nil
}

// evalHTMLAttrValueNodes evaluates a slice of AST nodes that appear inside an
// HTML attribute value and appends the resulting text to valParts. Control
// structures (if, with, range) are supported by walking them with the HTML
// node dispatcher and extracting text from the resulting html.Node slice.
func (s *state) evalHTMLAttrValueNodes(dot reflect.Value, nodes []Node, valParts *[]string) error {
	for _, v := range nodes {
		switch vn := v.(type) {
		case *TextNode:
			*valParts = append(*valParts, string(vn.Text))
		case *ActionNode:
			val, err := s.evalPipeline(dot, vn.Pipe)
			if err != nil {
				return err
			}
			if !val.IsValid() {
				*valParts = append(*valParts, "<no value>")
				continue
			}
			iface := printableValue(val)
			if iface == nil {
				return s.errorf("can't print %s of type %s", val, val.Type())
			}
			trust := s.trustOf(val)
			text := fmt.Sprint(iface)
			text, _ = escapeAction(text, vn.HTMLContext, trust)
			*valParts = append(*valParts, text)
		case *IfNode, *WithNode, *RangeNode:
			htmlNodes, err := s.walkHTMLNode(dot, vn)
			if err != nil {
				return err
			}
			for _, hn := range htmlNodes {
				*valParts = append(*valParts, htmlNodeText(hn))
			}
		}
	}
	return nil
}

func (s *state) walkHTMLAction(dot reflect.Value, action *ActionNode) ([]*html.Node, error) {
	action.count.Add(1)
	val, err := s.evalPipeline(dot, action.Pipe)
	if err != nil {
		return nil, err
	}
	if len(action.Pipe.Decl) > 0 {
		return nil, nil
	}
	if !val.IsValid() {
		return []*html.Node{{
			Type: html.TextNode,
			Data: "<no value>",
		}}, nil
	}
	iface := printableValue(val)
	if iface == nil {
		return nil, s.errorf("can't print %s of type %s", val, val.Type())
	}
	trust := s.trustOf(val)
	// A non-string value landing directly in a <script> expression
	// position (e.g. {{.Result}} where Result is a struct or slice, the
	// shape a JSON-hydration <script> tag takes) renders as a JS literal
	// via json.Marshal, matching html/template's jsValEscaper — not as
	// Go's %v syntax wrapped in a JS string, which is neither valid JS
	// for a composite value nor what the template author intended.
	// TrustJS/TrustJSStr are explicit opt-outs the escapeAction path
	// below already honors; a value under either trust kind is left to
	// that path so it is never double-escaped.
	if action.HTMLContext != nil && action.HTMLContext.State == "text" &&
		action.HTMLContext.Element == "script" && action.HTMLContext.JSContext == "expr" &&
		trust != TrustJS && trust != TrustJSStr {
		if text, ok := jsExprJSONMarshal(iface); ok {
			return []*html.Node{{Type: html.RawNode, Data: text}}, nil
		}
	}
	text := fmt.Sprint(iface)
	text, raw := escapeAction(text, action.HTMLContext, trust)
	if text == "" {
		return nil, nil
	}
	nodeType := html.TextNode
	if raw {
		nodeType = html.RawNode
	}
	return []*html.Node{{
		Type: nodeType,
		Data: text,
	}}, nil
}

func (s *state) walkHTMLIf(dot reflect.Value, n *IfNode) ([]*html.Node, error) {
	return s.walkHTMLBranch(dot, &n.BranchNode, "if")
}

func (s *state) walkHTMLWith(dot reflect.Value, n *WithNode) ([]*html.Node, error) {
	return s.walkHTMLBranch(dot, &n.BranchNode, "with")
}

func (s *state) walkHTMLBranch(dot reflect.Value, branch *BranchNode, name string) ([]*html.Node, error) {
	branch.openCount.Add(1)
	pipe, err := s.evalPipeline(dot, branch.Pipe)
	if err != nil {
		return nil, err
	}
	truth, ok := isTrue(indirectInterface(pipe))
	if !ok {
		return nil, s.errorf("if/with can't use %v", pipe)
	}
	var nodes []*html.Node
	if truth {
		newDot := dot
		if name == "with" {
			newDot = pipe
		}
		lastTrim := branch.EndTrimLeft
		if branch.ElseList != nil && len(branch.ElseList.Nodes) > 0 {
			lastTrim = branch.ElseTrimLeft
		}
		nodes, err = s.walkHTMLList(newDot, branch.List, branch.StartTrimRight, lastTrim)
	} else if branch.ElseList != nil {
		branch.elseCount.Add(1)
		nodes, err = s.walkHTMLList(dot, branch.ElseList, branch.ElseTrimRight, branch.EndTrimLeft)
	}
	if err != nil {
		return nodes, err
	}
	branch.endCount.Add(1)
	return nodes, nil
}

func (s *state) walkHTMLRange(dot reflect.Value, r *RangeNode) ([]*html.Node, error) {
	r.BranchNode.openCount.Add(1)
	val, err := s.evalPipeline(dot, r.Pipe)
	if err != nil {
		return nil, err
	}
	val, _ = indirect(val)
	mark := len(s.vars)
	var result []*html.Node
	oneIteration := func(index, elem reflect.Value) error {
		if len(r.Pipe.Decl) > 0 {
			if r.Pipe.IsAssign {
				if len(r.Pipe.Decl) > 1 {
					s.setVar(r.Pipe.Decl[0].Ident[0], index)
				}
				s.setVar(r.Pipe.Decl[len(r.Pipe.Decl)-1].Ident[0], elem)
			} else {
				s.setTopVar(1, elem)
				if len(r.Pipe.Decl) > 1 {
					s.setTopVar(2, index)
				}
			}
		}
		lastTrim := r.EndTrimLeft
		if r.ElseList != nil && len(r.ElseList.Nodes) > 0 {
			lastTrim = r.ElseTrimLeft
		}
		nodes, err := s.walkHTMLList(elem, r.List, r.StartTrimRight, lastTrim)
		result = append(result, nodes...)
		if err != nil {
			return err
		}
		return nil
	}
	switch val.Kind() {
	case reflect.Array, reflect.Slice:
		if val.Len() == 0 {
			break
		}
		for i := 0; i < val.Len(); i++ {
			if err := oneIteration(reflect.ValueOf(i), val.Index(i)); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					continue
				}
				return nil, err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Map:
		if val.Len() == 0 {
			break
		}
		iter := val.MapRange()
		for iter.Next() {
			if err := oneIteration(iter.Key(), iter.Value()); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					continue
				}
				return nil, err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		count := val.Int()
		if count <= 0 {
			break
		}
		for i := int64(0); i < count; i++ {
			if err := oneIteration(reflect.ValueOf(int(i)), reflect.ValueOf(int(i))); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					continue
				}
				return nil, err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		count := val.Uint()
		if count == 0 {
			break
		}
		for i := uint64(0); i < count; i++ {
			if err := oneIteration(reflect.ValueOf(int(i)), reflect.ValueOf(int(i))); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					continue
				}
				return nil, err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Chan:
		if val.IsNil() {
			break
		}
		i := 0
		for {
			elem, ok := val.Recv()
			if !ok {
				break
			}
			if err := oneIteration(reflect.ValueOf(i), elem); err != nil {
				if isBreak(err) {
					break
				}
				if isContinue(err) {
					i++
					continue
				}
				return nil, err
			}
			i++
		}
		if i == 0 {
			break
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Invalid:
		break
	default:
		return nil, s.errorf("range can't iterate over %v", val)
	}
	s.vars = s.vars[:mark]
	if r.ElseList != nil {
		r.BranchNode.elseCount.Add(1)
		nodes, walkErr := s.walkHTMLList(dot, r.ElseList, r.ElseTrimRight, r.EndTrimLeft)
		if walkErr != nil {
			return nodes, walkErr
		}
		r.BranchNode.endCount.Add(1)
		return nodes, nil
	}
	r.BranchNode.endCount.Add(1)
	return nil, nil
}

func (s *state) walkHTMLTemplate(dot reflect.Value, t *TemplateNode) ([]*html.Node, error) {
	t.count.Add(1)
	if s.depth >= maxExecDepth {
		return nil, s.errorf("exceeded maximum template depth (%d)", maxExecDepth)
	}
	if s.finder == nil {
		return nil, s.errorf("template %q not defined", t.Name)
	}
	tree, ok := s.finder.FindTree(t.Name)
	if !ok {
		return nil, s.errorf("template %q not defined", t.Name)
	}
	newDot := dot
	if t.Pipe != nil {
		var err error
		newDot, err = s.evalPipeline(dot, t.Pipe)
		if err != nil {
			return nil, err
		}
	}
	// Execute the template into a buffer and parse the result as HTML? No —
	// we walk the sub-template's tree and collect html.Nodes.
	newState := &state{
		tree:   tree,
		cp:     s.cp,
		depth:  s.depth + 1,
		funcs:  s.funcs,
		finder: s.finder,
		trust:  s.trust,
		vars:   []variable{{"$", newDot}},
	}
	s.cp.Init(tree)
	root := tree.Root
	if root == nil {
		return nil, nil
	}
	// Check if the sub-template is HTML (has an HTMLDocumentFragmentNode).
	if len(root.Nodes) == 1 {
		if frag, ok := root.Nodes[0].(*HTMLDocumentFragmentNode); ok {
			var result []*html.Node
			for _, child := range frag.Children {
				nodes, err := newState.walkHTMLNode(newDot, child)
				if err != nil {
					return nil, err
				}
				result = append(result, nodes...)
			}
			return result, nil
		}
	}
	// Check if the root contains HTML nodes (e.g., from extracted
	// {{define}}/{{block}} in HTML templates), or the tree was parsed in
	// HTML mode. Both require the HTML walker so contextual escaping is
	// applied to interpolated actions. tree.RootTrimFirst/RootTrimLast
	// carry the defining node's own trim markers (see executeHTMLList).
	if tree.HTMLMode || rootHasHTMLNodes(root) {
		return newState.walkHTMLList(newDot, root, tree.RootTrimFirst, tree.RootTrimLast)
	}
	// Plain text sub-template: render to string, return as text node.
	var buf bytes.Buffer
	newState.wr = &buf
	if err := newState.walk(newDot, root); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, nil
	}
	return []*html.Node{{
		Type: html.TextNode,
		Data: buf.String(),
	}}, nil
}

func (s *state) walkHTMLList(dot reflect.Value, list *ListNode, trimFirst, trimLast bool) ([]*html.Node, error) {
	var result []*html.Node
	nodes := list.Nodes
	for i, node := range nodes {
		if tn, ok := node.(*TextNode); ok {
			text := string(tn.Text)
			if (i == 0 && trimFirst) || (i > 0 && nodeRightTrim(nodes[i-1])) {
				text = strings.TrimLeft(text, trimCutset)
			}
			if (i == len(nodes)-1 && trimLast) || (i < len(nodes)-1 && nodeLeftTrim(nodes[i+1])) {
				text = strings.TrimRight(text, trimCutset)
			}
			tn.count.Add(1)
			if len(text) > 0 {
				result = append(result, &html.Node{
					Type: html.TextNode,
					Data: text,
				})
			}
			continue
		}
		htmlNodes, err := s.walkHTMLNode(dot, node)
		if err != nil {
			if isBreak(err) || isContinue(err) {
				result = append(result, htmlNodes...)
				return result, err
			}
			return nil, err
		}
		result = append(result, htmlNodes...)
	}
	return result, nil
}

// Attribute-position branch helpers for if/with inside element attributes.
func (s *state) walkHTMLAttrBranch(dot reflect.Value, branch *BranchNode, name string) ([]html.Attribute, error) {
	branch.openCount.Add(1)
	pipe, err := s.evalPipeline(dot, branch.Pipe)
	if err != nil {
		return nil, err
	}
	truth, ok := isTrue(indirectInterface(pipe))
	if !ok {
		return nil, s.errorf("if/with can't use %v", pipe)
	}
	var attrs []html.Attribute
	if truth {
		newDot := dot
		if name == "with" {
			newDot = pipe
		}
		attrs, err = s.walkHTMLAttrList(newDot, branch.List)
	} else if branch.ElseList != nil {
		branch.elseCount.Add(1)
		attrs, err = s.walkHTMLAttrList(dot, branch.ElseList)
	}
	if err != nil {
		return nil, err
	}
	branch.endCount.Add(1)
	return attrs, nil
}

func (s *state) walkHTMLAttrRange(dot reflect.Value, r *RangeNode) ([]html.Attribute, error) {
	r.BranchNode.openCount.Add(1)
	val, err := s.evalPipeline(dot, r.Pipe)
	if err != nil {
		return nil, err
	}
	val, _ = indirect(val)
	mark := len(s.vars)
	var result []html.Attribute
	oneIteration := func(index, elem reflect.Value) error {
		if len(r.Pipe.Decl) > 0 {
			if r.Pipe.IsAssign {
				if len(r.Pipe.Decl) > 1 {
					s.setVar(r.Pipe.Decl[0].Ident[0], index)
				}
				s.setVar(r.Pipe.Decl[len(r.Pipe.Decl)-1].Ident[0], elem)
			} else {
				s.setTopVar(1, elem)
				if len(r.Pipe.Decl) > 1 {
					s.setTopVar(2, index)
				}
			}
		}
		attrs, err := s.walkHTMLAttrList(elem, r.List)
		if err != nil {
			return err
		}
		result = append(result, attrs...)
		return nil
	}
	switch val.Kind() {
	case reflect.Array, reflect.Slice:
		if val.Len() == 0 {
			break
		}
		for i := 0; i < val.Len(); i++ {
			if err := oneIteration(reflect.ValueOf(i), val.Index(i)); err != nil {
				return nil, err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Map:
		if val.Len() == 0 {
			break
		}
		iter := val.MapRange()
		for iter.Next() {
			if err := oneIteration(iter.Key(), iter.Value()); err != nil {
				return nil, err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		count := val.Int()
		if count <= 0 {
			break
		}
		for i := int64(0); i < count; i++ {
			if err := oneIteration(reflect.ValueOf(int(i)), reflect.ValueOf(int(i))); err != nil {
				return nil, err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		count := val.Uint()
		if count == 0 {
			break
		}
		for i := uint64(0); i < count; i++ {
			if err := oneIteration(reflect.ValueOf(int(i)), reflect.ValueOf(int(i))); err != nil {
				return nil, err
			}
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Chan:
		if val.IsNil() {
			break
		}
		i := 0
		for {
			elem, ok := val.Recv()
			if !ok {
				break
			}
			if err := oneIteration(reflect.ValueOf(i), elem); err != nil {
				return nil, err
			}
			i++
		}
		if i == 0 {
			break
		}
		s.vars = s.vars[:mark]
		r.BranchNode.endCount.Add(1)
		return result, nil
	case reflect.Invalid:
		break
	default:
		return nil, s.errorf("range can't iterate over %v", val)
	}
	s.vars = s.vars[:mark]
	if r.ElseList != nil {
		r.BranchNode.elseCount.Add(1)
		attrs, walkErr := s.walkHTMLAttrList(dot, r.ElseList)
		if walkErr != nil {
			return nil, walkErr
		}
		r.BranchNode.endCount.Add(1)
		return attrs, nil
	}
	r.BranchNode.endCount.Add(1)
	return nil, nil
}

func (s *state) walkHTMLAttrList(dot reflect.Value, list *ListNode) ([]html.Attribute, error) {
	var result []html.Attribute
	for _, node := range list.Nodes {
		switch n := node.(type) {
		case *HTMLAttributeNode:
			attr, err := s.evalHTMLAttribute(dot, n)
			if err != nil {
				return nil, err
			}
			result = append(result, attr)
		case *IfNode:
			attrs, err := s.walkHTMLAttrBranch(dot, &n.BranchNode, "if")
			if err != nil {
				return nil, err
			}
			result = append(result, attrs...)
		case *WithNode:
			attrs, err := s.walkHTMLAttrBranch(dot, &n.BranchNode, "with")
			if err != nil {
				return nil, err
			}
			result = append(result, attrs...)
		case *ActionNode:
			// Action in attribute list context
			val, err := s.evalPipeline(dot, n.Pipe)
			if err != nil {
				return nil, err
			}
			if len(n.Pipe.Decl) > 0 {
				continue
			}
			if !val.IsValid() {
				continue
			}
			iface := printableValue(val)
			if iface == nil {
				return nil, s.errorf("can't print %s of type %s", val, val.Type())
			}
			text := fmt.Sprint(iface)
			if text != "" {
				result = append(result, html.Attribute{Val: text})
			}
		case *TextNode:
			// Whitespace between attributes — ignore.
		}
	}
	return result, nil
}

// htmlNodeText extracts the text content from an html.Node. For text nodes,
// this returns Data directly. For element nodes, it recursively extracts text
// from children, avoiding the bug where hn.Data would return the tag name.
func htmlNodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		buf.WriteString(htmlNodeText(c))
	}
	return buf.String()
}

// validateAttrName checks that an attribute name contains only valid characters.
// Per the HTML spec, attribute names must not contain spaces, quotes, >, /, =,
// or control characters.
func validateAttrName(name string) error {
	if name == "" {
		return fmt.Errorf("empty attribute name")
	}
	for _, r := range name {
		switch {
		case r == ' ', r == '\t', r == '\n', r == '\f', r == '\r':
			return fmt.Errorf("contains whitespace")
		case r == '"', r == '\'', r == '>', r == '/', r == '=':
			return fmt.Errorf("contains %q", r)
		case r <= 0x1f || r == 0x7f:
			return fmt.Errorf("contains control character")
		}
	}
	return nil
}

// Error helpers

func (s *state) errorf(format string, args ...any) ExecError {
	name := s.tree.Name
	return errorf(name, format, args...)
}

func (s *state) writeError(err error) ExecError {
	return s.errorf("write error: %w", err)
}

var reflectValueType = reflect.TypeFor[reflect.Value]()

// stripAttrQuotes removes surrounding single or double quotes from an attribute value text.
// The HTML parser stores attribute value text with the surrounding quotes;
// we need to strip them for the html.Attribute.Val field.
func stripAttrQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// validateArg converts an argument value for a function call.
// If the parameter type is reflect.Value, wrap the arg.
// Otherwise, unwrap interfaces.
func (s *state) validateArg(paramType reflect.Type, arg reflect.Value) reflect.Value {
	if paramType == reflectValueType {
		return reflect.ValueOf(arg)
	}
	if !arg.IsValid() {
		if paramType != nil && canBeNil(paramType) {
			return reflect.Zero(paramType)
		}
		return arg
	}
	if arg.Kind() == reflect.Interface && !arg.IsNil() {
		arg = arg.Elem()
	}
	return arg
}

// missingVal is the zero reflect.Value used to signal "missing".
var missingVal reflect.Value

// Sentinel errors for break/continue in range loops.
type (
	breakError    struct{}
	continueError struct{}
)

func (breakError) Error() string    { return "break" }
func (continueError) Error() string { return "continue" }

func isBreak(err error) bool {
	_, ok := err.(breakError)
	return ok
}

func isContinue(err error) bool {
	_, ok := err.(continueError)
	return ok
}

// Unused import suppression removed — atom is used in walkHTMLElement.
var _ = atom.Lookup
