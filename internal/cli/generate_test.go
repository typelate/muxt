package cli

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGenerate(t *testing.T) {
	t.Run("unknown flag", func(t *testing.T) {
		_, err := NewRoutesFileConfiguration([]string{
			"--unknown",
		}, io.Discard)
		assert.ErrorContains(t, err, "flag provided but not defined")
	})
	t.Run(ReceiverStaticType+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := NewRoutesFileConfiguration([]string{
			"--" + ReceiverStaticType, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(routesFunc+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := NewRoutesFileConfiguration([]string{
			"--" + routesFunc, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(templatesVariable+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := NewRoutesFileConfiguration([]string{
			"--" + templatesVariable, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(outputFlagName+" flag value is not a go file", func(t *testing.T) {
		_, err := NewRoutesFileConfiguration([]string{
			"--" + outputFlagName, "output.txt",
		}, io.Discard)
		assert.ErrorContains(t, err, "filename must use .go extension")
	})
}
