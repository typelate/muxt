package mutation

import (
	"slices"
	"strings"
	"text/template/parse"
)

// OperatorCondition forces one condition of a boolean decision, leaving
// every other condition reading the template's real data.
//
// Forcing every condition at once only ever produces the then branch or
// the else branch, which if-true and if-false already cover. Forcing one
// leaves a decision that is still a function of the data, so whether a
// test notices depends on the data it renders with: that is what shows
// whether anything is coupled to that condition in particular.
const OperatorCondition Operator = "condition"

// OperatorConditionDead marks a condition that boolean simplification
// proved cannot change the decision, so no test could ever be coupled to
// it and no mutant is worth running.
const OperatorConditionDead Operator = "condition-dead"

type boolKind int

const (
	boolCond boolKind = iota
	boolConst
	boolNot
	boolAnd
	boolOr
)

// boolNode is a decision written with and, or and not over conditions
// this package can locate in the source.
type boolNode struct {
	kind  boolKind
	value bool
	text  string
	kids  []*boolNode
}

// decision builds the boolean structure of a pipeline.
//
// It reports false for anything it cannot model exactly -- a function
// call, a comparison, a pipeline of several commands -- because a
// half-understood decision would produce mutants that claim more than
// they test.
func decision(text string, pipe *parse.PipeNode) (*boolNode, bool) {
	if pipe == nil || len(pipe.Cmds) != 1 {
		return nil, false
	}
	return decisionCommand(text, pipe.Cmds[0])
}

func decisionCommand(text string, command *parse.CommandNode) (*boolNode, bool) {
	if command == nil || len(command.Args) == 0 {
		return nil, false
	}
	if ident, ok := command.Args[0].(*parse.IdentifierNode); ok {
		kind, ok := logicalKind(ident.Ident)
		if !ok {
			return nil, false
		}
		return decisionCall(kind, text, command.Args[1:])
	}
	if len(command.Args) != 1 {
		return nil, false
	}
	return decisionArg(text, command.Args[0])
}

func logicalKind(function string) (boolKind, bool) {
	switch function {
	case "and":
		return boolAnd, true
	case "or":
		return boolOr, true
	case "not":
		return boolNot, true
	default:
		return boolCond, false
	}
}

func decisionCall(kind boolKind, text string, args []parse.Node) (*boolNode, bool) {
	if len(args) == 0 || (kind == boolNot && len(args) != 1) {
		return nil, false
	}
	kids := make([]*boolNode, 0, len(args))
	for _, arg := range args {
		kid, ok := decisionArg(text, arg)
		if !ok {
			return nil, false
		}
		kids = append(kids, kid)
	}
	return &boolNode{kind: kind, kids: kids}, true
}

func decisionArg(text string, arg parse.Node) (*boolNode, bool) {
	switch node := arg.(type) {
	case *parse.PipeNode:
		return decision(text, node)
	case *parse.BoolNode:
		return &boolNode{kind: boolConst, value: node.True}, true
	case *parse.FieldNode, *parse.VariableNode, *parse.DotNode:
		var found []operand
		appendOperand(&found, text, arg, nil)
		if len(found) != 1 {
			return nil, false
		}
		return &boolNode{kind: boolCond, text: found[0].text}, true
	default:
		return nil, false
	}
}

// canonical renders a node so that two structurally equal decisions
// compare equal, which is what the simplification rules are stated over.
func (n *boolNode) canonical() string {
	switch n.kind {
	case boolCond:
		return n.text
	case boolConst:
		if n.value {
			return "true"
		}
		return "false"
	case boolNot:
		return "not(" + n.kids[0].canonical() + ")"
	default:
		parts := make([]string, 0, len(n.kids))
		for _, kid := range n.kids {
			parts = append(parts, kid.canonical())
		}
		slices.Sort(parts)
		name := "and("
		if n.kind == boolOr {
			name = "or("
		}
		return name + strings.Join(parts, ",") + ")"
	}
}

// simplify rewrites a decision to an equivalent one with as few
// conditions as possible.
//
// Fewer conditions is fewer test runs, and a condition that simplification
// removes is one no test could have been coupled to.
func (n *boolNode) simplify() *boolNode {
	switch n.kind {
	case boolCond, boolConst:
		return n
	case boolNot:
		return negate(n.kids[0].simplify())
	default:
		return n.simplifyJunction()
	}
}

func constant(value bool) *boolNode { return &boolNode{kind: boolConst, value: value} }

func negate(kid *boolNode) *boolNode {
	switch kid.kind {
	case boolConst:
		return constant(!kid.value)
	case boolNot:
		return kid.kids[0]
	default:
		return &boolNode{kind: boolNot, kids: []*boolNode{kid}}
	}
}

// simplifyJunction simplifies an and or an or. Its zero is the constant
// that decides the whole decision, false for and and true for or, and its
// identity, the opposite, contributes nothing.
func (n *boolNode) simplifyJunction() *boolNode {
	zero := n.kind == boolOr
	kids, decided := foldConstants(flatten(n.kind, n.kids), zero)
	if decided {
		return constant(zero)
	}
	kids = distinct(kids)
	if hasComplement(kids) {
		return constant(zero)
	}
	kids = absorb(kids, n.kind)

	switch len(kids) {
	case 0:
		return constant(!zero)
	case 1:
		return kids[0]
	default:
		return &boolNode{kind: n.kind, kids: kids}
	}
}

// flatten simplifies kids and lifts the children of any that has kind.
func flatten(kind boolKind, kids []*boolNode) []*boolNode {
	flat := make([]*boolNode, 0, len(kids))
	for _, kid := range kids {
		kid = kid.simplify()
		if kid.kind == kind {
			flat = append(flat, kid.kids...)
			continue
		}
		flat = append(flat, kid)
	}
	return flat
}

// foldConstants drops the constants that are the junction's identity, and
// reports true when one is its zero.
func foldConstants(kids []*boolNode, zero bool) ([]*boolNode, bool) {
	kept := make([]*boolNode, 0, len(kids))
	for _, kid := range kids {
		switch {
		case kid.kind != boolConst:
			kept = append(kept, kid)
		case kid.value == zero:
			return nil, true
		}
	}
	return kept, false
}

func distinct(kids []*boolNode) []*boolNode {
	seen := make(map[string]struct{}, len(kids))
	kept := make([]*boolNode, 0, len(kids))
	for _, kid := range kids {
		key := kid.canonical()
		if _, done := seen[key]; done {
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, kid)
	}
	return kept
}

// hasComplement reports whether kids hold both X and not X.
func hasComplement(kids []*boolNode) bool {
	seen := make(map[string]struct{}, len(kids))
	for _, kid := range kids {
		seen[kid.canonical()] = struct{}{}
	}
	for _, kid := range kids {
		if kid.kind != boolNot {
			continue
		}
		if _, found := seen[kid.kids[0].canonical()]; found {
			return true
		}
	}
	return false
}

// absorb drops a child that another child already implies: X and (X or Y)
// is X, and X or (X and Y) is X.
func absorb(kids []*boolNode, kind boolKind) []*boolNode {
	opposite := boolOr
	if kind == boolOr {
		opposite = boolAnd
	}

	simple := make(map[string]struct{})
	for _, kid := range kids {
		if kid.kind != opposite {
			simple[kid.canonical()] = struct{}{}
		}
	}

	kept := kids[:0:0]
	for _, kid := range kids {
		if kid.kind == opposite && absorbed(kid, simple) {
			continue
		}
		kept = append(kept, kid)
	}
	return kept
}

func absorbed(node *boolNode, simple map[string]struct{}) bool {
	for _, kid := range node.kids {
		if _, found := simple[kid.canonical()]; found {
			return true
		}
	}
	return false
}

// conditions returns the distinct conditions a decision reads, in the
// order they are first written.
func (n *boolNode) conditions() []string {
	var (
		found []string
		seen  = make(map[string]struct{})
	)
	var walk func(*boolNode)
	walk = func(node *boolNode) {
		if node == nil {
			return
		}
		if node.kind == boolCond {
			if _, done := seen[node.text]; !done {
				seen[node.text] = struct{}{}
				found = append(found, node.text)
			}
			return
		}
		for _, kid := range node.kids {
			walk(kid)
		}
	}
	walk(n)
	return found
}

// addConditions appends a mutant per condition and truth value, and
// records the conditions simplification proved cannot matter.
func (e *enumerator) addConditions(a action) bool {
	r := a.region
	tree, ok := decision(e.src.text, a.pipe)
	if !ok {
		return false
	}

	written := tree.conditions()
	if len(written) < 2 {
		// A single condition is what if-true and if-false already force.
		return false
	}

	live := tree.simplify().conditions()
	ops := operands(e.src.text, a.dot, a.pipe)

	for _, name := range written {
		spans := occurrences(ops, name)
		if len(spans) == 0 {
			continue
		}
		if !slices.Contains(live, name) {
			e.appendEdits(r, OperatorConditionDead, spans2edits(spans, "false"), name+" cannot change the decision")
			continue
		}
		for _, value := range [...]string{"false", "true"} {
			e.appendEdits(r, OperatorCondition, spans2edits(spans, value), name+"="+value)
		}
	}
	return true
}

func occurrences(ops []operand, text string) []operand {
	var found []operand
	for _, op := range ops {
		if op.text == text {
			found = append(found, op)
		}
	}
	return found
}

func spans2edits(ops []operand, value string) []edit {
	edits := make([]edit, 0, len(ops))
	for _, op := range ops {
		edits = append(edits, edit{start: op.start, end: op.end, text: value})
	}
	return edits
}
