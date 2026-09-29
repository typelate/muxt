package analysis

import (
	"go/token"
	"go/types"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, tt.want, got, "newReferences() names")
		})
	}

	t.Run("references are sorted by position and typed", func(t *testing.T) {
		got := newReferences("example.com/p", refs(), nil)[1]
		require.Len(t, got.References, 2, "references of b = %+v, want offsets 1 then 9", got.References)
		assert.Equal(t, 1, got.References[0].Position.Offset, "references of b = %+v, want offsets 1 then 9", got.References)
		assert.Equal(t, 9, got.References[1].Position.Offset, "references of b = %+v, want offsets 1 then 9", got.References)
		assert.Equal(t, "string", got.References[0].Data, "Data")
	})
}

func TestNewNamedReferencesOrdersKindsAtOnePosition(t *testing.T) {
	pos := token.Position{Filename: "a.gohtml", Offset: 4}
	parse := TemplateReference{Name: "x", Kind: ParseTemplateNode, Position: pos, data: types.Typ[types.Int]}
	execute := TemplateReference{Name: "x", Kind: ExecuteTemplateNode, Position: pos, data: types.Typ[types.Int]}
	for _, in := range [][]TemplateReference{{parse, execute}, {execute, parse}} {
		var kinds []TemplateReferenceKind
		for _, ref := range NewNamedReferences("example.com/p", "x", in).References {
			kinds = append(kinds, ref.Kind)
		}
		assert.Equal(t, []TemplateReferenceKind{ExecuteTemplateNode, ParseTemplateNode}, kinds, "NewNamedReferences(%v) kinds, want execute_template then template", in)
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
			assert.Len(t, got.References, tt.want, "NewNamedReferences() references: %+v", got.References)
		})
	}
}
