package mutation

import (
	"fmt"
	"go/types"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/asteval"
)

// variant is what a mutant does, without where.
type variant struct {
	operator Operator
	detail   string
}

// variants describes mutants by operator and detail. An operands mutant's
// values are drawn, so it is described by which operands it replaced.
func variants(mutants []Mutant) []variant {
	var got []variant
	for _, m := range mutants {
		detail := m.detail
		if m.Operator == OperatorOperands {
			var names []string
			for _, part := range strings.Fields(detail) {
				name, _, _ := strings.Cut(part, "=")
				names = append(names, name)
			}
			detail = strings.Join(names, " ")
		}
		got = append(got, variant{operator: m.Operator, detail: detail})
	}
	return got
}

// enumerate lists the mutants of a template "t" holding body, rendered
// with dot.
func enumerate(t *testing.T, body string, dot types.Type, maxCases int) ([]Mutant, []budgetNote) {
	t.Helper()
	text := `{{define "t"}}` + body + `{{end}}`
	trees, err := asteval.ParseTrees("t.gohtml", text, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	src := newFileSource("t.gohtml", "t.gohtml", text, "", "")
	sc := scope{template: "t", dataType: dot, treeLocation: treeLocation{src: src, tree: trees["t"]}}
	return mutantsInScope(sc, nil, newValues(1), maxCases)
}

// TestMutantsInScope states which variations each kind of action gets, in
// the order a report lists them: by where they start, then by operator.
func TestMutantsInScope(t *testing.T) {
	page := dataType(t, pageSource, "Page")
	for _, tt := range []struct {
		name string
		body string
		want []variant
	}{
		{name: "a field", body: `{{.Name}}`, want: []variant{{OperatorActionZero, `""`}}},
		{
			name: "a declaration keeps its variable and loses its value",
			body: `{{$x := .Count}}{{$x}}`,
			want: []variant{{OperatorActionZero, "0"}, {OperatorActionEmpty, `""`}},
		},
		{
			name: "two inputs are varied in every combination",
			body: `{{printf "%s %d" .Name .Count}}`,
			want: []variant{
				{OperatorActionZero, `""`},
				{OperatorOperands, ".Name"},
				{OperatorOperands, ".Name .Count"},
				{OperatorOperands, ".Count"},
			},
		},
		{name: "an if", body: `{{if .Flag}}x{{end}}`, want: []variant{{OperatorIfFalse, "false"}, {OperatorIfTrue, "true"}}},
		{
			name: "a decision forces one condition at a time",
			body: `{{if and .Flag .Name}}x{{end}}`,
			want: []variant{
				{OperatorIfFalse, "false"},
				{OperatorIfTrue, "true"},
				{OperatorCondition, ".Flag=false"},
				{OperatorCondition, ".Flag=true"},
				{OperatorCondition, ".Name=false"},
				{OperatorCondition, ".Name=true"},
			},
		},
		{
			name: "a condition that cannot change the decision",
			body: `{{if or .Flag (and .Flag .Name)}}x{{end}}`,
			want: []variant{
				{OperatorIfFalse, "false"},
				{OperatorIfTrue, "true"},
				{OperatorCondition, ".Flag=false"},
				{OperatorCondition, ".Flag=true"},
				{OperatorConditionDead, ".Name cannot change the decision"},
			},
		},
		{name: "a range", body: `{{range .Items}}x{{else}}none{{end}}`, want: []variant{{OperatorRangeNever, "none"}}},
		{
			name: "a with, and its body with the dot it selects",
			body: `{{with .Owner}}{{.Email}}{{end}}`,
			want: []variant{{OperatorWithEmpty, ""}, {OperatorActionZero, `""`}},
		},
		{name: "a template call", body: `{{template "row" .Name}}`, want: []variant{{OperatorTemplateDrop, ""}}},
		{name: "a block is not dropped", body: `{{block "row" .Name}}{{.}}{{end}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mutants, notes := enumerate(t, tt.body, page, DefaultMaxCases)
			if got := variants(mutants); !slices.Equal(got, tt.want) {
				t.Errorf("mutants:\n got %q\nwant %q", got, tt.want)
			}
			if len(notes) != 0 {
				t.Errorf("notes = %v, want none", notes)
			}
		})
	}
}

// TestMutantsInScopeHoldsBackAnActionOverBudget states that an action with
// more combinations than --max-cases allows contributes none of them, and
// says so where the action is.
func TestMutantsInScopeHoldsBackAnActionOverBudget(t *testing.T) {
	mutants, notes := enumerate(t, `{{printf "%s %d" .Name .Count}}`, dataType(t, pageSource, "Page"), 2)
	if got, want := variants(mutants), []variant{{OperatorActionZero, `""`}}; !slices.Equal(got, want) {
		t.Errorf("mutants %q, want %q", got, want)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want one", notes)
	}
	note := notes[0]
	if note.line != 1 || note.column != 15 || note.template != "t" {
		t.Errorf("note at %s %d:%d, want t 1:15", note.template, note.line, note.column)
	}
	if got, want := note.reason(), "2 operands need 3 cases, over --max-cases=2"; got != want {
		t.Errorf("reason = %q, want %q", got, want)
	}
}

// TestValuesDraw states what a substituted value looks like: a literal of
// the operand's own type where there is one, and a quoted word otherwise.
func TestValuesDraw(t *testing.T) {
	word := regexp.MustCompile(`^"[a-z]{4}"$`)
	isWord := func(s string) error {
		if !word.MatchString(s) {
			return fmt.Errorf("not a quoted four letter word")
		}
		return nil
	}
	for _, tt := range []struct {
		name  string
		typ   types.Type
		valid func(string) error
	}{
		{name: "a string", typ: types.Typ[types.String], valid: isWord},
		{name: "a safe string", typ: safeHTML(), valid: isWord},
		{name: "no type", typ: nil, valid: isWord},
		{name: "a struct", typ: types.NewStruct(nil, nil), valid: isWord},
		{name: "a complex number", typ: types.Typ[types.Complex128], valid: isWord},
		{name: "a bool", typ: types.Typ[types.Bool], valid: func(s string) error { _, err := strconv.ParseBool(s); return err }},
		{name: "an int", typ: types.Typ[types.Int], valid: func(s string) error { _, err := strconv.Atoi(s); return err }},
		{name: "a float", typ: types.Typ[types.Float64], valid: func(s string) error { _, err := strconv.ParseFloat(s, 64); return err }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := newValues(1)
			for range 20 {
				if drawn := v.draw(tt.typ); tt.valid(drawn) != nil {
					t.Fatalf("draw = %q, which is not a literal of %v", drawn, tt.typ)
				}
			}
		})
	}
}

// TestValuesAreSeeded states that the same seed draws the same values, so
// a report can be repeated, and a different seed draws different ones.
func TestValuesAreSeeded(t *testing.T) {
	draws := func(seed uint64) []string {
		v := newValues(seed)
		var out []string
		for range 8 {
			out = append(out, v.draw(types.Typ[types.String]), v.draw(types.Typ[types.Int]))
		}
		return out
	}
	if !slices.Equal(draws(7), draws(7)) {
		t.Error("the same seed drew different values")
	}
	if slices.Equal(draws(7), draws(8)) {
		t.Error("different seeds drew the same values")
	}
}
