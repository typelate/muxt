package analysis

import (
	"go/token"
	"go/types"
	"regexp"
	"testing"
)

func TestNewReferences(t *testing.T) {
	ref := func(name string, offset int) TemplateReference {
		return TemplateReference{
			Name:     name,
			Kind:     ParseTemplateNode,
			Position: token.Position{Filename: "a.gohtml", Offset: offset},
			data:     types.Typ[types.String],
		}
	}
	refs := func() map[string][]TemplateReference {
		return map[string][]TemplateReference{
			"b": {ref("b", 9), ref("b", 1)},
			"a": {ref("a", 3)},
			"c": {ref("c", 5)},
		}
	}
	names := func(named []NamedReferences) []string {
		var got []string
		for _, n := range named {
			got = append(got, n.Name)
		}
		return got
	}

	for _, tt := range []struct {
		name   string
		filter []*regexp.Regexp
		want   []string
	}{
		{name: "no filter lists every name in order", want: []string{"a", "b", "c"}},
		{name: "a filter matching a subset", filter: []*regexp.Regexp{regexp.MustCompile(`^[ac]$`)}, want: []string{"a", "c"}},
		{name: "any of several filters", filter: []*regexp.Regexp{regexp.MustCompile(`^a$`), regexp.MustCompile(`^b$`)}, want: []string{"a", "b"}},
		{name: "a filter matching nothing", filter: []*regexp.Regexp{regexp.MustCompile(`zzz`)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := names(newReferences("example.com/p", refs(), tt.filter))
			if len(got) != len(tt.want) {
				t.Fatalf("newReferences() names = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("newReferences() names = %v, want %v", got, tt.want)
				}
			}
		})
	}

	t.Run("references are sorted by position and typed", func(t *testing.T) {
		got := newReferences("example.com/p", refs(), nil)[1]
		if len(got.References) != 2 || got.References[0].Position.Offset != 1 || got.References[1].Position.Offset != 9 {
			t.Fatalf("references of b = %+v, want offsets 1 then 9", got.References)
		}
		if got.References[0].Data != "string" {
			t.Errorf("Data = %q, want %q", got.References[0].Data, "string")
		}
	})
}

func TestNewNamedReferencesOrdersKindsAtOnePosition(t *testing.T) {
	pos := token.Position{Filename: "a.gohtml", Offset: 4}
	parse := TemplateReference{Name: "x", Kind: ParseTemplateNode, Position: pos, data: types.Typ[types.Int]}
	execute := TemplateReference{Name: "x", Kind: ExecuteTemplateNode, Position: pos, data: types.Typ[types.Int]}
	for _, in := range [][]TemplateReference{{parse, execute}, {execute, parse}} {
		got := NewNamedReferences("example.com/p", "x", in).References
		if len(got) != 2 || got[0].Kind != ExecuteTemplateNode || got[1].Kind != ParseTemplateNode {
			t.Errorf("NewNamedReferences(%v) kinds = %v, want execute_template then template", in, got)
		}
	}
}

func TestNewNamedReferences(t *testing.T) {
	pos := token.Position{Filename: "a.gohtml", Offset: 4}
	tp := types.Typ[types.Int]
	for _, tt := range []struct {
		name string
		refs []TemplateReference
		want int
	}{
		{name: "identical references collapse", refs: []TemplateReference{
			{Name: "x", Kind: ParseTemplateNode, Position: pos, data: tp},
			{Name: "x", Kind: ParseTemplateNode, Position: pos, data: tp},
		}, want: 1},
		{name: "a different kind at the same position stays", refs: []TemplateReference{
			{Name: "x", Kind: ParseTemplateNode, Position: pos, data: tp},
			{Name: "x", Kind: ExecuteTemplateNode, Position: pos, data: tp},
		}, want: 2},
		{name: "a different type at the same position stays", refs: []TemplateReference{
			{Name: "x", Kind: ParseTemplateNode, Position: pos, data: tp},
			{Name: "x", Kind: ParseTemplateNode, Position: pos, data: types.Typ[types.String]},
		}, want: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := NewNamedReferences("example.com/p", "x", tt.refs)
			if len(got.References) != tt.want {
				t.Errorf("NewNamedReferences() has %d references, want %d: %+v", len(got.References), tt.want, got.References)
			}
		})
	}
}
