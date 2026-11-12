package scaffold

import "strconv"

// Body is what a new define holds: the choice between a template that
// contributes a fragment to a page and one that is a page.
type Body int

const (
	// Fragment is an empty define, for a route that renders part of a page.
	Fragment Body = iota
	// Document is a define holding an empty HTML document, for a route that
	// renders a whole page.
	Document
)

// document is the page a Document define holds, indented as muxt fmt leaves it.
const document = `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title></title>
</head>
<body></body>
</html>
`

// Definition returns the define action that adds the route template name to a
// template file, holding body.
func Definition(name string, body Body) string {
	content := ""
	if body == Document {
		content = document
	}
	return "{{define " + strconv.Quote(name) + "}}" + content + "{{end}}"
}

// AppendDefinition returns existing with define added after it: on its own
// line, and with the file ending in one newline.
func AppendDefinition(existing []byte, define string) []byte {
	out := append([]byte(nil), existing...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, define...)
	return append(out, '\n')
}
