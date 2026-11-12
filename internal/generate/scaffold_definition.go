package generate

import "strconv"

// DefineBody is what a new define holds: the choice between a template that
// contributes a fragment to a page and one that is a page.
type DefineBody int

const (
	// FragmentBody is an empty define, for a route that renders part of a page.
	FragmentBody DefineBody = iota
	// DocumentBody is a define holding an empty HTML document, for a route that
	// renders a whole page.
	DocumentBody
)

// document is the page a DocumentBody define holds, indented as muxt fmt leaves it.
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

// DefineBlock returns the define action that adds the route template name to a
// template file, holding body.
func DefineBlock(name string, body DefineBody) string {
	content := ""
	if body == DocumentBody {
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
