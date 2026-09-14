package analysis_test

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/analysis"
)

// TestListingConfigurationJSON states that a listing's configuration reads
// and writes as JSON, patterns included, which is how an archive in
// testdata holds the configuration it runs with.
func TestListingConfigurationJSON(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config analysis.TemplateCallersConfiguration
		want   string
	}{
		{
			name:   "no patterns",
			config: analysis.TemplateCallersConfiguration{TemplatesVariables: []string{"templates"}},
			want:   `{"TemplatesVariables":["templates"],"FilterTemplates":null}`,
		},
		{
			name:   "a pattern is the text it was written as",
			config: analysis.TemplateCallersConfiguration{TemplatesVariables: []string{"pages"}, FilterTemplates: []*regexp.Regexp{regexp.MustCompile("^head")}},
			want:   `{"TemplatesVariables":["pages"],"FilterTemplates":["^head"]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			written, err := json.Marshal(tt.config)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(written))

			var read analysis.TemplateCallersConfiguration
			require.NoError(t, json.Unmarshal(written, &read))
			assert.Equal(t, tt.config, read)
		})
	}
}

// TestListingConfigurationJSONRejectsABadPattern states that a pattern that
// does not compile is reported when the configuration is read.
func TestListingConfigurationJSONRejectsABadPattern(t *testing.T) {
	var config analysis.TemplateCallsConfiguration
	err := json.Unmarshal([]byte(`{"FilterTemplates":["("]}`), &config)
	require.ErrorContains(t, err, "FilterTemplates: error parsing regexp")
}
