package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAppendDefinitionStartsDefineOnItsOwnLine states that a define added to a
// template file starts on its own line and leaves the file ending in one newline.
func TestAppendDefinitionStartsDefineOnItsOwnLine(t *testing.T) {
	const define = `{{define "GET /foo Index()"}}{{end}}`
	for _, tt := range []struct {
		name     string
		existing string
		want     string
	}{
		{name: "empty file", existing: "", want: define + "\n"},
		{name: "ends with a newline", existing: "{{define \"a\"}}{{end}}\n", want: "{{define \"a\"}}{{end}}\n" + define + "\n"},
		{name: "does not end with a newline", existing: "{{define \"a\"}}{{end}}", want: "{{define \"a\"}}{{end}}\n" + define + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := AppendDefinition([]byte(tt.existing), define)

			assert.Equal(t, tt.want, string(got), "AppendDefinition(%q)", tt.existing)
		})
	}
}

// TestDefinitionDocumentHoldsAnHTMLDocument states the define for a full
// page: the doctype, a head with a charset, a viewport and an empty title, and
// an empty body, indented as muxt fmt leaves it.
func TestDefinitionDocumentHoldsAnHTMLDocument(t *testing.T) {
	const want = `{{define "GET /foo Index()"}}<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title></title>
</head>
<body></body>
</html>
{{end}}`

	got := Definition("GET /foo Index()", Document)

	assert.Equal(t, want, got, "Definition(name, Document)")
}
