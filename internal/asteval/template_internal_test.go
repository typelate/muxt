package asteval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseTemplateNames(t *testing.T) {
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
			result := parseTemplateNames(tt.input)
			assert.EqualValues(t, tt.expected, result)
		})
	}

	t.Run("mismatched backtick", func(t *testing.T) {
		// TODO: make parseTemplateNames return an error
		assert.NotPanics(t, func() {
			_ = parseTemplateNames("`x\"")
		})
	})

	t.Run("mismatched double quote", func(t *testing.T) {
		// TODO: make parseTemplateNames return an error
		assert.NotPanics(t, func() {
			_ = parseTemplateNames("\"x`")
		})
	})
}
