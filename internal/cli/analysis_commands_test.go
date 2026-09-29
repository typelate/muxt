package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilePatterns(t *testing.T) {
	t.Run("none stays nil", func(t *testing.T) {
		got, err := compilePatterns(nil)
		require.NoError(t, err, "compilePatterns(nil)")
		require.Nil(t, got, "compilePatterns(nil)")
	})
	t.Run("in order", func(t *testing.T) {
		got, err := compilePatterns([]string{`^GET `, `Home$`})
		require.NoError(t, err, "compilePatterns()")
		require.Len(t, got, 2, "compilePatterns() want both patterns in order")
		assert.Equal(t, `^GET `, got[0].String())
		assert.Equal(t, `Home$`, got[1].String())
	})
	t.Run("invalid", func(t *testing.T) {
		_, err := compilePatterns([]string{`ok`, `(`})
		require.EqualError(t, err, "error parsing regexp: missing closing ): `(`", "compilePatterns()")
	})
}

func TestCheckTemplatesVariables(t *testing.T) {
	for _, tt := range []struct {
		name    string
		in      []string
		wantErr string
	}{
		{name: "none"},
		{name: "identifiers", in: []string{"templates", "_pages2"}},
		{name: "empty is left to fixTemplateVariables", in: []string{""}},
		{name: "not an identifier", in: []string{"ok", "not-ok"}, wantErr: "variable not-ok value must be a well-formed Go identifier"},
		{name: "keyword", in: []string{"range"}, wantErr: "variable range value must be a well-formed Go identifier"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTemplatesVariables(tt.in)
			if tt.wantErr == "" {
				require.NoError(t, err, "checkTemplatesVariables(%q)", tt.in)
				return
			}
			require.EqualError(t, err, tt.wantErr, "checkTemplatesVariables(%q)", tt.in)
		})
	}
}
