package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_parsePatterns(t *testing.T) {
	for _, tt := range []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "quoted globs with double quotes",
			input:    `*.txt "*.md" "images/*.png"`,
			expected: []string{"*.txt", "*.md", "images/*.png"},
		},
		{
			name:     "quoted globs with backticks",
			input:    "*.go `*.js` `css/*.css`",
			expected: []string{"*.go", "*.js", "css/*.css"},
		},
		{
			name:     "glob with spaces",
			input:    `"file with spaces.txt"`,
			expected: []string{"file with spaces.txt"},
		},
		{
			name:     "unclosed quote",
			input:    `"unclosed quote`,
			expected: []string{"unclosed quote"},
		},
		{
			name:     "plain files",
			input:    "plain `other`",
			expected: []string{"plain", "other"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePatterns(tt.input)
			require.NoError(t, err)
			assert.EqualValues(t, tt.expected, result)
		})
	}
}
