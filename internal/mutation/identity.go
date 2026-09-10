package mutation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/types"
	"hash"
	"text/template/parse"

	"github.com/typelate/check"
)

// identity is everything an action's mutants depend on.
//
// Two runs that reach the same identity for an action are asking the
// tests the same question, so the earlier answer still holds.
type identity struct {
	// engine is the muxt version that produced the mutants. An
	// improvement to the engine can change what a template is mutated
	// into, so verdicts do not carry across one.
	engine string

	// template is the name the action's template is rendered under, and
	// dot the type it is rendered with. The two together are the
	// execution a verdict belongs to.
	template string
	dot      string

	// source is the template's own source: for a defined template, the
	// text from {{define}} through {{end}}; for the template a file
	// carries, what its definitions leave behind.
	source string

	// types are the types resolved for every action in the template.
	// They are recorded for the template rather than the action so that
	// a field changing type re-runs the template as a whole, which is
	// the unit a reader works in.
	types string

	// seed decides the values substituted, so verdicts reached under one
	// are not claimed for another.
	seed uint64
}

// fingerprint identifies one action's mutants.
//
// The action's own text is not enough to tell it from an identically
// written one elsewhere in the same template, so its place in the walk
// goes in too.
func (id identity) fingerprint(action string, index int) string {
	h := sha256.New()
	fmt.Fprintf(h, "v%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%s\x00%d\x00",
		stateVersion, id.engine, id.template, id.dot, id.source, id.types, id.seed, action, index)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// typeDigest accumulates the types a template's actions read.
//
// The types are written as the walk reaches them, so the digest depends
// on the whole template rather than on any one action: a field changing
// type re-runs all of it. That is deliberate -- a template is the unit a
// reader works in, and a run that re-ran only the actions naming the
// changed field would leave the rest claiming verdicts reached under the
// old type.
type typeDigest struct {
	hash hash.Hash
}

func newTypeDigest() *typeDigest {
	return &typeDigest{hash: sha256.New()}
}

// observe records one action's types.
func (d *typeDigest) observe(text string, a action) {
	fmt.Fprintf(d.hash, "%d\x00%s\x00", a.index, typeKey(a.dot))
	for _, op := range a.operands(text) {
		// The resolution goes in beside the type: a method's signature
		// can change while its first result stays put, and a template
		// reading it as a field stops working when it gains a parameter.
		fmt.Fprintf(d.hash, "%s=%s=%s\x00", op.text, typeKey(op.dataType), op.resolution)
	}
}

func (d *typeDigest) sum() string {
	return hex.EncodeToString(d.hash.Sum(nil))
}

// scan walks a template once and reports what both the mutants and the
// identifiers are built from: every action, and the identity they share.
//
// Callers get the actions in the order they were reached, which is the
// order they are mutated in, and an identity that already accounts for
// every one of them.
type scan struct {
	Actions  []action
	Identity identity
}

// scanTemplate walks a template and gathers what identifying and
// mutating it both need.
func scanTemplate(src *templateSource, tree *parse.Tree, dot types.Type, functions check.Functions, source string, run runIdentity) scan {
	var (
		actions []action
		digest  = newTypeDigest()
	)
	walkActions(src.text, src.regions, dot, functions, tree.Root, func(a action) {
		digest.observe(src.text, a)
		actions = append(actions, a)
	})
	return scan{
		Actions: actions,
		Identity: identity{
			engine:   run.engine,
			template: tree.Name,
			dot:      typeKey(dot),
			source:   source,
			types:    digest.sum(),
			seed:     run.seed,
		},
	}
}

// runIdentity is what every action in a run shares: the values a mutation
// draws from and the engine that drew them.
//
// They travel together because they invalidate together -- a change to
// either means the mutants themselves are different -- and passing them
// as one value keeps them from being reordered on the way through.
//
// The test suite is deliberately not here. It does not change what a
// mutant is, only whether an old verdict about one still holds, which is
// a question the state answers per result.
type runIdentity struct {
	seed   uint64
	engine string
}
