package mutation

import (
	"fmt"
	"go/types"
	"path/filepath"
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

// Identifiers calculates what a run may reuse a verdict for.
//
// An identifier answers "has anything changed that could change the
// verdict". Four things can:
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
//   - The seed and the muxt version, which decide what a mutation
//     substitutes and which mutations exist at all.
//
// It holds a cache, so a partial reached from twenty entrypoints with the
// same dot is read, walked and hashed once. The cache belongs to one set
// of definitions and must not be carried across an edit to them, which is
// why it is reached only through a constructor.
type Identifiers struct {
	defs      []check.Definition
	functions check.Functions
	seed      uint64
	engine    string

	// sources is the same reader a run uses, so "what is this
	// template's own source" is answered once, in one place.
	sources *sourceCollector
	read    map[string]struct{}
	srcOf   map[string]*templateSource

	busy map[string]struct{}
	done map[string]Identifier
}

// NewIdentifiers prepares to identify executions of one loaded template
// set.
//
// seed and engine must be the ones a run would use, or the identifiers
// will not match the fingerprints that run records.
func NewIdentifiers(defs []check.Definition, functions check.Functions, seed uint64, engine string) *Identifiers {
	return &Identifiers{
		defs:      defs,
		functions: functions,
		seed:      seed,
		engine:    engine,
		// No packages: a template written as a Go string literal cannot
		// be located without the loaded package, and source refuses one
		// rather than answering wrongly.
		sources: newSourceCollector("", nil),
		read:    make(map[string]struct{}),
		srcOf:   make(map[string]*templateSource),
		busy:    make(map[string]struct{}),
		done:    make(map[string]Identifier),
	}
}

// Identify calculates the tree of identifiers for one template
// execution: the entrypoint rendered with dot, every action in it, and
// the executions it reaches through {{template}}.
func (ids *Identifiers) Identify(entrypoint *parse.Tree, dot types.Type) (Identifier, error) {
	if entrypoint == nil || entrypoint.Root == nil {
		return Identifier{}, fmt.Errorf("identify: no tree to identify")
	}
	return ids.template(entrypoint, dot)
}

func (ids *Identifiers) template(tree *parse.Tree, dot types.Type) (Identifier, error) {
	key := executionKey(tree.Name, dot)
	if cached, ok := ids.done[key]; ok {
		return cached, nil
	}
	if _, running := ids.busy[key]; running {
		// A template that reaches itself, directly or through a partial.
		// Its identifier is already being computed further up; naming it
		// here is enough to record that it is reached.
		return Identifier{Name: tree.Name, DataType: typeKey(dot)}, nil
	}
	ids.busy[key] = struct{}{}
	defer delete(ids.busy, key)

	src, digests, err := ids.source(tree.Name)
	if err != nil {
		return Identifier{}, err
	}

	// One walk, and one source digest rule, shared with the run: the same
	// actions, in the same order, identified the same way.
	scanned := scanTemplate(src, tree, dot, ids.functions, digests[tree.Name], ids.seed, ids.engine)

	identifier := Identifier{
		Name:     tree.Name,
		DataType: typeKey(dot),
		Sum:      scanned.Identity.fingerprint("", 0),
	}
	for _, a := range scanned.Actions {
		identifier.Actions = append(identifier.Actions, Identifier{
			Name: a.text,
			Sum:  scanned.Identity.fingerprint(a.text, a.index),
		})
	}

	cyclic := false
	within := make(map[string]struct{})
	for _, a := range scanned.Actions {
		call, ok := a.invocation(src.text, ids.functions)
		if !ok {
			continue
		}
		child := executionKey(call.name, call.dot)
		if _, listed := within[child]; listed {
			// One template may invoke a partial several times with the
			// same value. That is one execution, listed once.
			continue
		}
		within[child] = struct{}{}

		definition, ok := definitionNamed(ids.defs, call.name)
		if !ok || definition.Tree == nil || definition.Tree.Root == nil {
			continue
		}
		if _, running := ids.busy[child]; running {
			cyclic = true
		}
		reached, err := ids.template(definition.Tree, call.dot)
		if err != nil {
			return Identifier{}, err
		}
		identifier.Templates = append(identifier.Templates, reached)
	}

	if !cyclic {
		// A template holding a cycle carries a stub for the partner it
		// could not follow, and which partner that is depends on where
		// the walk started. Caching it would hand a later, differently
		// rooted walk an answer shaped by the first one.
		ids.done[key] = identifier
	}
	return identifier, nil
}

// source returns the text a template was written in, and a digest for
// every template that text defines.
//
// The digests come from the run's own collector, so an identifier
// calculated here is the one a run records; there is no second rule that
// could drift from it. A template written as a Go string literal is
// refused rather than answered wrongly: its parse tree is positioned
// against the decoded literal, and mapping that back through the
// escaping needs the loaded package, which a run has and this does not.
func (ids *Identifiers) source(name string) (*templateSource, map[string]string, error) {
	own, ok := definitionNamed(ids.defs, name)
	if !ok {
		return nil, nil, fmt.Errorf("identify: no definition named %q", name)
	}
	file := own.Define.Position.Filename
	if filepath.Ext(file) == ".go" {
		return nil, nil, fmt.Errorf("identify: %q is written as a Go string literal, which needs the loaded package to locate", name)
	}

	if _, done := ids.read[file]; !done {
		// Every definition the file holds, not just this one: a
		// template's source is what the others carve out of the text,
		// so a digest taken before they are all filed would describe a
		// template that still contains them.
		ids.read[file] = struct{}{}
		for _, definition := range ids.defs {
			if definition.Define.Position.Filename != file {
				continue
			}
			src, err := ids.sources.add(definition)
			if err != nil {
				return nil, nil, fmt.Errorf("identify: %w", err)
			}
			if src != nil {
				ids.srcOf[definition.Name] = src
			}
		}
	}

	src, ok := ids.srcOf[name]
	if !ok {
		return nil, nil, fmt.Errorf("identify: %q was not written in %s", name, file)
	}
	return src, ids.sources.digests(src), nil
}

func definitionNamed(defs []check.Definition, name string) (check.Definition, bool) {
	for _, definition := range defs {
		if definition.Name == name {
			return definition, true
		}
	}
	return check.Definition{}, false
}

// executionKey names a template rendered with one type of dot, which is
// the unit a verdict belongs to.
//
// Neither a template name nor a type's string can hold a NUL, so no two
// pairs run together into one key.
func executionKey(name string, dot types.Type) string {
	return name + "\x00" + typeKey(dot)
}
