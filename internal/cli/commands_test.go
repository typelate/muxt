package cli

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/generate"
)

func TestFixTemplateVariables(t *testing.T) {
	t.Run("empty slice gets the default", func(t *testing.T) {
		got := []string(nil)
		require.NoError(t, fixTemplateVariables(&got, ""))
		assert.Equal(t, []string{defaultTemplatesVariableName}, got)
	})

	t.Run("default slice with deprecated flag adopts the deprecated value", func(t *testing.T) {
		got := []string{defaultTemplatesVariableName}
		require.NoError(t, fixTemplateVariables(&got, "myTemplates"))
		assert.Equal(t, []string{"myTemplates"}, got)
	})

	t.Run("deprecated flag combined with non-default new flag is rejected", func(t *testing.T) {
		got := []string{"adminTemplates", "publicTemplates"}
		err := fixTemplateVariables(&got, "legacyTemplates")
		require.Error(t, err)
		assert.Contains(t, err.Error(), deprecatedTemplatesVariable)
		assert.Contains(t, err.Error(), useTemplatesVariable)
	})

	t.Run("explicit user values pass through", func(t *testing.T) {
		got := []string{"adminTemplates", "publicTemplates"}
		require.NoError(t, fixTemplateVariables(&got, ""))
		assert.Equal(t, []string{"adminTemplates", "publicTemplates"}, got)
	})

	t.Run("duplicate user values are rejected", func(t *testing.T) {
		got := []string{"adminTemplates", "adminTemplates"}
		err := fixTemplateVariables(&got, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate template variable")
		assert.Contains(t, err.Error(), "adminTemplates")
	})

	t.Run("empty user value is rejected", func(t *testing.T) {
		got := []string{""}
		err := fixTemplateVariables(&got, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), useTemplatesVariable)
		assert.Contains(t, err.Error(), "must not be empty")
	})
}

func TestFindDuplicateVariables(t *testing.T) {
	t.Run("no duplicates returns nil", func(t *testing.T) {
		require.NoError(t, findDuplicateVariables([]string{"a", "b", "c"}))
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		require.NoError(t, findDuplicateVariables(nil))
	})

	t.Run("duplicate is reported", func(t *testing.T) {
		err := findDuplicateVariables([]string{"a", "b", "a"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "a")
	})
}

func TestByteSize_Set(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		want    int64
		wantErr string
	}{
		{name: "decimal MB", input: "32MB", want: 32_000_000},
		{name: "binary MiB", input: "32MiB", want: 32 << 20},
		{name: "GB", input: "1GB", want: 1_000_000_000},
		{name: "bare bytes", input: "1024", want: 1024},
		{name: "empty", input: "", wantErr: "invalid byte size"},
		{name: "zero", input: "0", wantErr: "must be positive"},
		{name: "garbage", input: "not-a-size", wantErr: "invalid byte size"},
		{name: "negative", input: "-1MB", wantErr: "invalid byte size"},
		{name: "overflow int64", input: "10EB", wantErr: "exceeds int64 maximum"},
		{name: "largest int64", input: "9223372036854775807", want: math.MaxInt64},
		{name: "one past int64", input: "9223372036854775808", wantErr: "exceeds int64 maximum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &generate.RoutesFileConfiguration{}
			f := byteSize{p: &cfg.MultipartMaxMemory}
			err := f.Set(tc.input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr, "Set(%q)", tc.input)
				return
			}
			require.NoError(t, err, "Set(%q)", tc.input)
			require.Equal(t, tc.want, cfg.MultipartMaxMemory, "Set(%q) stored", tc.input)
		})
	}
}

func TestByteSize_String(t *testing.T) {
	t.Run("zero shows default", func(t *testing.T) {
		f := byteSize{p: new(int64)}
		got := f.String()
		require.True(t, strings.Contains(got, "MiB") || strings.Contains(got, "MB"), "String() = %q, want a human-readable size", got)
	})
	t.Run("a nil pointer shows default", func(t *testing.T) {
		want := byteSize{p: new(int64)}.String()
		assert.Equal(t, want, byteSize{}.String(), "String() of a byteSize with no pointer")
	})
	t.Run("override shows override", func(t *testing.T) {
		n := int64(64 << 20)
		f := byteSize{p: &n}
		require.Contains(t, f.String(), "64", "String()")
	})
}
