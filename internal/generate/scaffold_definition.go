package generate

import (
	"fmt"
	"strconv"

	"github.com/ettle/strcase"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
	"github.com/typelate/muxt/template/html"
)

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

// RouteFileBase returns the identifier muxt gives the route name, in snake case.
// The identifier is the one muxt uses for the route's generated names, so it is
// asked of muxt rather than derived from the name here.
func RouteFileBase(name string) (string, error) {
	templates, err := html.New("").Parse(DefineBlock(name, FragmentBody))
	if err != nil {
		return "", fmt.Errorf("parsing %q: %w", name, err)
	}
	defs, err := muxt.Definitions(source.Variable{Name: "probe", Set: templates})
	if err != nil {
		return "", err
	}
	if len(defs) != 1 {
		return "", fmt.Errorf("%q is not one route definition", name)
	}
	return strcase.ToSnake(defs[0].Identifier()), nil
}
