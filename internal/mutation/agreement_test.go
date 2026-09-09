package mutation

import "testing"

// TestIdentifyAgreesWithTheMutantsARunProduces states the contract the
// whole reuse mechanism rests on: the identifier calculated ahead of a
// run is the fingerprint that run records.
//
// The two are reached by different routes -- Identify from a set of
// definitions, a run from a scope its loader built -- and nothing else
// in the suite compares them. If they drift, every run recalculates
// everything and reports it as work saved, or worse, hands a verdict to
// an action that never earned it. Both symptoms are silent.
func TestIdentifyAgreesWithTheMutantsARunProduces(t *testing.T) {
	const text = `<h1>{{.Title}}</h1>
{{define "page"}}{{if .On}}{{.Name}}{{else}}{{.Other}}{{end}}{{range .Items}}{{.}}{{end}}{{end}}
`
	const (
		seed   = 7
		engine = "test-engine"
	)
	defs, trees := identifyFixture(t, text)

	identified, err := NewIdentifiers(defs, nil, seed, engine).Identify(trees["page"], nil)
	if err != nil {
		t.Fatal(err)
	}
	sums := make(map[string]string, len(identified.Actions))
	for _, a := range identified.Actions {
		sums[a.Sum] = a.Name
	}
	if len(sums) != len(identified.Actions) {
		t.Fatalf("identifiers = %d for %d actions, want one each", len(sums), len(identified.Actions))
	}

	// The scope is assembled the way a run assembles one: through the
	// collector, not through anything Identify touched. That is what
	// makes this an agreement rather than a restatement.
	collector := newSourceCollector("", nil)
	var src *templateSource
	for _, definition := range defs {
		filed, err := collector.add(definition)
		if err != nil {
			t.Fatal(err)
		}
		if definition.Name == "page" {
			src = filed
		}
	}
	if src == nil {
		t.Fatal("the collector filed no source for the template under test")
	}
	sc := scope{
		template:     "page",
		src:          src,
		tree:         trees["page"],
		sourceDigest: collector.digests(src)["page"],
	}

	mutants, _ := mutantsInScope(sc, nil, newValues(seed), DefaultMaxCases, seed, engine)
	if len(mutants) == 0 {
		t.Fatal("the run produced no mutants, so this proves nothing")
	}

	reached := make(map[string]struct{})
	for _, m := range mutants {
		name, ok := sums[m.Fingerprint()]
		if !ok {
			t.Errorf("the run fingerprinted %q as %s, which Identify never calculated", m.Action(), m.Fingerprint())
			continue
		}
		if name != m.Action() {
			t.Errorf("fingerprint %s is %q to the run and %q to Identify", m.Fingerprint(), m.Action(), name)
		}
		reached[m.Fingerprint()] = struct{}{}
	}

	// Every action Identify names must be one a run actually mutates.
	// An identifier for something no mutant carries is state written for
	// a question that is never asked, and it would never be invalidated.
	for sum, name := range sums {
		if _, ok := reached[sum]; !ok {
			t.Errorf("Identify calculated %s for %q, which the run mutates nothing for", sum, name)
		}
	}
}
