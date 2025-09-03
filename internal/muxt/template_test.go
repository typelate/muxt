package muxt_test

import (
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/muxt"
)

func TestTemplates(t *testing.T) {
	t.Run("when one of the template names is a malformed pattern", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "HEAD /"}}{{end}}`))
		_, err := muxt.Templates(ts)
		require.Error(t, err)
	})
	t.Run("when the pattern is not unique", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{define "GET  / F1()"}}a{{end}} {{define "GET /  F2()"}}b{{end}}`))
		_, err := muxt.Templates(ts)
		require.Error(t, err)
	})
}
