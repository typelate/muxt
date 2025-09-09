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
		assert.ErrorContains(t, err, "flag provided but not defined")
	})
	t.Run(receiverStaticType+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + receiverStaticType, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(routesFunc+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + routesFunc, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(templatesVariable+" flag value is an invalid identifier", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + templatesVariable, "123",
		}, io.Discard)
		assert.ErrorContains(t, err, errIdentSuffix)
	})
	t.Run(outputFlagName+" flag value is not a go file", func(t *testing.T) {
		_, err := newRoutesFileConfiguration([]string{
			"--" + outputFlagName, "output.txt",
		}, io.Discard)
		assert.ErrorContains(t, err, "filename must use .go extension")
	})
}
