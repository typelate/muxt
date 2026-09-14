package mutation_test

import (
	"encoding/json/v2"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/configjson"
	"github.com/typelate/muxt/internal/mutation"
)

// TestConfigurationJSON states how a run's configuration reads and writes
// as JSON, which is how an archive in testdata holds the one it plans with:
// a pattern is the text the command line wrote, and a flag the command line
// left alone is null. A pattern read as "" would be a pattern matching
// everything, which is not what leaving --run alone means, so the archives
// hold null.
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
			written, err := json.Marshal(tt.config, configjson.Options())
			require.NoError(t, err)

			var read mutation.Configuration
			require.NoError(t, json.Unmarshal(written, &read, configjson.Options()))
			assert.Equal(t, tt.config, read)
		})
	}
}

// TestConfigurationJSONRejectsABadPattern states that a pattern that does
// not compile is reported where it was read.
func TestConfigurationJSONRejectsABadPattern(t *testing.T) {
	var config mutation.Configuration
	err := json.Unmarshal([]byte(`{"TemplatePattern":"("}`), &config, configjson.Options())
	require.ErrorContains(t, err, "error parsing regexp")
	require.ErrorContains(t, err, "TemplatePattern")
}
