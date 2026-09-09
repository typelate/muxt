package mutation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/types"
	"hash"
	"os"
	"slices"
	"strings"
	"text/template/parse"

	"github.com/typelate/check"
)

// Identifier names one thing a verdict may be reused for: a template
// rendered with a type of dot, and each action within it.
//
// Two runs that produce the same identifier for an action are asking the
// same question of the tests, so the earlier answer still holds.
type Identifier struct {
	// Name is the template's name, or for an action the action as it is
	// written.
	Name string

	// DataType is the type of dot the template is rendered with, empty
	// for an action. The same template rendered with two types is two
	// executions, asking the tests two different questions.
	DataType string

	// Sum identifies it.
	Sum string

	// Actions are the identifiers of the actions the template holds, in
	// the order a walk reaches them, which is the order they are
	// mutated in.
	Actions []Identifier

	// Templates are the executions this one reaches through {{template}},
	// each with the type of dot it passes on.
	//
	// A template reached more than once with the same type appears once:
	// the second reaches the same identifier, so computing it again would
	// only repeat the answer.
	Templates []Identifier
}

// Identify calculates the tree of identifiers for one template
// execution: the entrypoint rendered with dot, and every action in it.
//
// An identifier answers "has anything changed that could change the
// verdict". Three things can:
//
//   - The template's own source. A defined template is the text from
//     {{define}} through {{end}}; the template a file carries is what its
//     definitions leave behind, since a change inside one cannot alter
//     what the surrounding template renders. The exception is the trim
//     markers on a definition's own delimiters, which act on the text
//     around the block, so those are kept.
//   - The types every action reads. A field going from a string to an int
//     changes what a mutation substitutes without changing a byte of the
//     template, and a method's signature can change while its first
//     result stays put.
//   - Where the action sits, since two identically written actions are
//     two mutants and must not share an answer.
//
// defs are the definitions of the source the entrypoint was written in;
// they are what separates one template's source from the text around it.
// functions may be nil, in which case a pipeline that calls a function
// resolves no type.
// cache may be nil. Passing one across calls is what keeps a template
// set cheap to identify: a partial reached from twenty entrypoints with
// the same dot is read, walked and hashed once.
func Identify(defs []check.Definition, entrypoint *parse.Tree, dot types.Type, functions check.Functions, cache IdentifierCache) (Identifier, error) {
	if entrypoint == nil || entrypoint.Root == nil {
		return Identifier{}, fmt.Errorf("identify: no tree to identify")
	}
	if cache == nil {
		cache = make(IdentifierCache)
	}
	scan := &identifyScan{
		defs:      defs,
		functions: functions,
		files:     make(map[string]string),
		busy:      make(map[string]struct{}),
		cache:     cache,
	}
	return scan.template(entrypoint, dot)
}

// IdentifierCache remembers executions already identified, keyed by the
// template and the type of dot it is rendered with.
//
// It is safe to reuse across the entrypoints of one load, and must not
// outlive the source it was built from: an edit changes what a template
// identifies as, which is the whole point of the identifier.
type IdentifierCache map[string]Identifier

// identifyScan carries what the walk shares: the definitions, the files
// they were read from, and the executions already identified.
type identifyScan struct {
	defs      []check.Definition
	functions check.Functions
	files     map[string]string
	busy      map[string]struct{}
	cache     IdentifierCache
}

func (s *identifyScan) read(path string) (string, error) {
	if text, ok := s.files[path]; ok {
		return text, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("identify: %w", err)
	}
	s.files[path] = string(b)
	return string(b), nil
}

func (s *identifyScan) template(tree *parse.Tree, dot types.Type) (Identifier, error) {
	key := tree.Name + "\x00" + typeKey(dot)
	if cached, ok := s.cache[key]; ok {
		return cached, nil
	}
	if _, running := s.busy[key]; running {
		// A template that reaches itself, directly or through a partial.
		// Its identifier is already being computed further up; naming it
		// here is enough to record that it is reached.
		return Identifier{Name: tree.Name, DataType: typeKey(dot)}, nil
	}
	s.busy[key] = struct{}{}
	defer delete(s.busy, key)

	own, ok := definitionNamed(s.defs, tree.Name)
	if !ok {
		return Identifier{}, fmt.Errorf("identify: no definition named %q", tree.Name)
	}
	text, err := s.read(own.Define.Position.Filename)
	if err != nil {
		return Identifier{}, err
	}
	source, err := definitionDigest(s.defs, own, text)
	if err != nil {
		return Identifier{}, err
	}

	// The types are only complete once the whole template has been
	// walked, so the actions are collected first and identified after:
	// every action shares the digest, and a type change anywhere in the
	// template re-runs all of it.
	var (
		found  []actionSite
		nested []templateCall
		seq    int
		digest = sha256.New()
	)
	walkActions(text, dot, s.functions, tree.Root, &seq, digest, &found, &nested)

	id := identity{
		template: tree.Name,
		dot:      typeKey(dot),
		source:   source,
		types:    hex.EncodeToString(digest.Sum(nil)),
	}

	identifier := Identifier{
		Name:     tree.Name,
		DataType: typeKey(dot),
		Sum:      id.fingerprint("", 0),
	}
	for _, site := range found {
		identifier.Actions = append(identifier.Actions, Identifier{
			Name: site.text,
			Sum:  id.fingerprint(site.text, site.index),
		})
	}

	within := make(map[string]struct{}, len(nested))
	for _, call := range nested {
		child := call.name + "\x00" + typeKey(call.dot)
		if _, done := within[child]; done {
			// One template may invoke a partial several times with the
			// same value. That is one execution, listed once.
			continue
		}
		within[child] = struct{}{}

		definition, ok := definitionNamed(s.defs, call.name)
		if !ok || definition.Tree == nil || definition.Tree.Root == nil {
			continue
		}
		reached, err := s.template(definition.Tree, call.dot)
		if err != nil {
			return Identifier{}, err
		}
		identifier.Templates = append(identifier.Templates, reached)
	}

	s.cache[key] = identifier
	return identifier, nil
}

// actionSite is one action reached by the walk.
type actionSite struct {
	index int
	text  string
}

func definitionNamed(defs []check.Definition, name string) (check.Definition, bool) {
	for _, definition := range defs {
		if definition.Name == name {
			return definition, true
		}
	}
	return check.Definition{}, false
}

// definitionDigest hashes the source that defines a template.
//
// The spans are hashed where they lie: nothing keeps a copy of a
// template's source, since only the digest is ever compared.
func definitionDigest(defs []check.Definition, own check.Definition, text string) (string, error) {
	start, end := own.Define.Offset, own.End.Offset+own.End.Length
	if start < 0 || end > len(text) || start > end {
		return "", fmt.Errorf("identify: %q is not within its file", own.Name)
	}

	if own.TemplateName.IsValid() {
		// A defined template is its own source, delimiters included.
		return digestOf(text[start:end]), nil
	}

	// The template the text carries is what the definitions leave. Their
	// trim markers stay, because those act on the text around the block.
	nested := make([]check.Definition, 0, len(defs))
	for _, definition := range defs {
		if definition.Name != own.Name && definition.TemplateName.IsValid() {
			nested = append(nested, definition)
		}
	}
	slices.SortFunc(nested, func(a, b check.Definition) int { return a.Define.Offset - b.Define.Offset })

	outer := sha256.New()
	last := start
	for _, definition := range nested {
		from, to := definition.Define.Offset, definition.End.Offset+definition.End.Length
		if from < last || to > end {
			continue
		}
		writeString(outer, text[last:from])
		fmt.Fprintf(outer, "\x00define %s %t %t\x00", definition.Name,
			strings.HasPrefix(text[from:min(from+definition.Define.Length, len(text))], "{{-"),
			strings.HasSuffix(text[definition.End.Offset:to], "-}}"))
		last = to
	}
	writeString(outer, text[last:end])
	return hex.EncodeToString(outer.Sum(nil)), nil
}

// walkActions reaches every action in the order the engine mutates them,
// carrying the type of dot in force at each one.
//
// It mirrors the walk that produces mutants, and the two must agree: an
// identifier taken over a different set of actions would let a change go
// unnoticed. TestIdentifyMatchesMutants holds them to it.
func walkActions(text string, dot types.Type, functions check.Functions, node parse.Node, seq *int, digest hash.Hash, out *[]actionSite, nested *[]templateCall) {
	found := regions(text, "", "")

	var walk func(parse.Node, types.Type)
	record := func(pipe *parse.PipeNode, at int, dot types.Type) {
		*seq++
		fmt.Fprintf(digest, "%d\x00%s\x00", *seq, typeKey(dot))
		for _, op := range operands(text, dot, pipe) {
			fmt.Fprintf(digest, "%s=%s=%s\x00", op.text, typeKey(op.dataType), op.resolution)
		}
		if _, r, ok := regionAt(found, at); ok {
			*out = append(*out, actionSite{index: *seq, text: text[r.start:r.end]})
		}
	}

	walk = func(node parse.Node, dot types.Type) {
		switch n := node.(type) {
		case *parse.ListNode:
			if n == nil {
				return
			}
			for _, child := range n.Nodes {
				walk(child, dot)
			}
		case *parse.ActionNode:
			record(n.Pipe, int(n.Pipe.Position()), dot)
		case *parse.IfNode:
			record(n.Pipe, int(n.Pipe.Position()), dot)
			walk(n.List, dot)
			walk(n.ElseList, dot)
		case *parse.WithNode:
			record(n.Pipe, int(n.Pipe.Position()), dot)
			walk(n.List, withDot(dot, n.Pipe, functions))
			walk(n.ElseList, dot)
		case *parse.RangeNode:
			record(n.Pipe, int(n.Pipe.Position()), dot)
			walk(n.List, rangeDot(dot, n.Pipe, functions))
			walk(n.ElseList, dot)
		case *parse.TemplateNode:
			record(n.Pipe, int(n.Position()), dot)
			// A {{template}} with no argument renders with no dot at
			// all, which is a different execution from one passing a
			// value along.
			passed := types.Type(nil)
			if n.Pipe != nil {
				if resolved, ok := pipelineType(dot, n.Pipe, functions); ok {
					passed = resolved
				}
			}
			*nested = append(*nested, templateCall{name: n.Name, dot: passed})
		}
	}
	walk(node, dot)
}
