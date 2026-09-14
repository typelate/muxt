package analysis_test

import (
	"encoding/json/v2"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/configjson"
)

// TestListingConfigurationJSON states how a listing's configuration reads
// and writes as JSON, which is how an archive in testdata holds the one it
// runs with: a --match pattern is the text it was written as, and a field
// the command line left alone is null rather than an empty list, so a
// configuration read back is the one the command line produced.
// regexp.Regexp reads and writes itself, as a TextMarshaler; configjson
// says the rest.
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
			written, err := json.Marshal(tt.config, configjson.Options())
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(written))

			var read analysis.TemplateCallersConfiguration
			require.NoError(t, json.Unmarshal(written, &read, configjson.Options()))
			assert.Equal(t, tt.config, read)
		})
	}
}

// TestListingConfigurationJSONRejectsABadPattern states that a pattern
// that does not compile is reported where it was read.
func TestListingConfigurationJSONRejectsABadPattern(t *testing.T) {
	var config analysis.TemplateCallsConfiguration
	err := json.Unmarshal([]byte(`{"FilterTemplates":["("]}`), &config, configjson.Options())
	require.ErrorContains(t, err, "error parsing regexp")
	require.ErrorContains(t, err, "FilterTemplates")
}
