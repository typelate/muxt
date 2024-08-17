package muxt

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTemplateName_ByPathThenMethod(t *testing.T) {
	for _, tt := range []struct {
		Name    string
		In, Exp []Pattern
	}{
		{
			Name: "sort by path then method",
			In: mustNewTemplateName(
				"GET /b",
				"POST /a",
				"GET /a",
			),
			Exp: mustNewTemplateName(
				"GET /a",
				"POST /a",
				"GET /b",
			),
		},
		{
			Name: "sort just paths",
			In: mustNewTemplateName(
				"/b",
				"/c",
				"/a",
			),
			Exp: mustNewTemplateName(
				"/a",
				"/b",
				"/c",
			),
		},
		{
			Name: "sort just methods",
			In: mustNewTemplateName(
				"DELETE /",
				"POST /",
				"GET /",
				"PATCH /",
			),
			Exp: mustNewTemplateName(
				"DELETE /",
				"GET /",
				"PATCH /",
				"POST /",
			),
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			slices.SortFunc(tt.In, Pattern.byPathThenMethod)
			assert.Equal(t, tt.Exp, tt.In)
		})
	}
}

func mustNewTemplateName(in ...string) []Pattern {
	var result []Pattern
	for _, n := range in {
		p, err, _ := NewPattern(n)
		if err != nil {
			panic(err)
		}
		result = append(result, p)
	}
	return result
}
