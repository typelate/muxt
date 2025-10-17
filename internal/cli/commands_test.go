package cli

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGenerate(t *testing.T) {
	t.Run("unknown flag", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--unknown",
		}, io.Discard)
		assert.ErrorContains(t, err, "unknown flag")
	})

	// Tests for new flag names
	t.Run(useReceiverType+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + useReceiverType, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(outputRoutesFunc+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + outputRoutesFunc, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(useTemplatesVariable+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + useTemplatesVariable, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(outputFile+" flag value is not a go file", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + outputFile, "output.txt",
		}, io.Discard)
		assert.ErrorContains(t, err, "filename must use .go extension")
	})

	// Tests for deprecated flags (backward compatibility)
	t.Run("deprecated "+deprecatedReceiverType+" flag still works", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + deprecatedReceiverType, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run("deprecated "+deprecatedRoutesFunc+" flag still works", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + deprecatedRoutesFunc, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run("deprecated "+deprecatedTemplatesVariable+" flag still works", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + deprecatedTemplatesVariable, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
}
