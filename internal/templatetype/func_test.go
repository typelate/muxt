package templatetype_test

import (
	"text/template"
	"text/template/parse"

	"github.com/crhntr/muxt/internal/templatetype"
)

func findTextTree(tmpl *template.Template) templatetype.FindTreeFunc {
	return func(name string) (*parse.Tree, bool) {
		ts := tmpl.Lookup(name)
		if ts == nil {
			return nil, false
		}
		return ts.Tree, true
	}
}
