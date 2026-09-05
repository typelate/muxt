package astgen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/astgen"
)

func TestNearestString(t *testing.T) {
	for _, tt := range []struct {
		Name       string
		Target     string
		Candidates []string
		Suggestion string
		Found      bool
	}{
		{
			Name:       "one edit away",
			Target:     "UserFeeld",
			Candidates: []string{"UserField", "UserName"},
			Suggestion: "UserField",
			Found:      true,
		},
		{
			Name:       "wrong case",
			Target:     "userfield",
			Candidates: []string{"UserField"},
			Suggestion: "UserField",
			Found:      true,
		},
		{
			Name:       "too far to look like a typo",
			Target:     "banana",
			Candidates: []string{"request", "response"},
			Found:      false,
		},
		{
			Name:       "short names allow one edit",
			Target:     "ct",
			Candidates: []string{"ctx"},
			Suggestion: "ctx",
			Found:      true,
		},
		{
			Name:       "no candidates",
			Target:     "anything",
			Candidates: nil,
			Found:      false,
		},
		{
			Name:       "picks the closest candidate",
			Target:     "reqest",
			Candidates: []string{"response", "request"},
			Suggestion: "request",
			Found:      true,
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			suggestion, found := astgen.NearestString(tt.Target, tt.Candidates)
			assert.Equal(t, tt.Found, found, "NearestString(%q, %v)", tt.Target, tt.Candidates)
			if tt.Found {
				assert.Equal(t, tt.Suggestion, suggestion)
			}
		})
	}
}
