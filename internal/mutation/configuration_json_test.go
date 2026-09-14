package mutation_test

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/mutation"
)

// TestConfigurationJSON states that a run's configuration reads and writes
// as JSON, patterns included, which is how an archive in testdata holds the
// configuration it plans with.
func TestConfigurationJSON(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config mutation.Configuration
	}{
		{
			name:   "no patterns",
			config: mutation.Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Seed: 1, SeedSet: true, MaxCases: mutation.DefaultMaxCases, Workers: 1},
		},
		{
			name:   "a pattern is the text it was written as",
			config: mutation.Configuration{TemplatesVariables: []string{"templates"}, TemplatePattern: regexp.MustCompile("^footer$"), Run: regexp.MustCompile("TestPage"), Packages: []string{"./..."}, MaxCases: 2, Workers: 4, Diff: "main", GoTestArgs: []string{"-count=1"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			written, err := json.Marshal(tt.config)
			require.NoError(t, err)

			var read mutation.Configuration
			require.NoError(t, json.Unmarshal(written, &read))
			assert.Equal(t, tt.config, read)
		})
	}
}

// TestConfigurationJSONRejectsABadPattern states that a pattern that does
// not compile is reported when the configuration is read.
func TestConfigurationJSONRejectsABadPattern(t *testing.T) {
	var config mutation.Configuration
	err := json.Unmarshal([]byte(`{"TemplatePattern":"("}`), &config)
	require.ErrorContains(t, err, "TemplatePattern: error parsing regexp")
}
