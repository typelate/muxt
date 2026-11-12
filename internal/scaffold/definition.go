package scaffold

import "strconv"

// Definition returns the define action that adds the route template name to a
// template file.
func Definition(name string) string {
	return "{{define " + strconv.Quote(name) + "}}{{end}}"
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
