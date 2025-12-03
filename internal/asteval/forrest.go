package asteval

import (
	"html/template"
	"text/template/parse"
)

type Forrest template.Template

func NewForrest(templates *template.Template) *Forrest {
	return (*Forrest)(templates)
}

func (f *Forrest) FindTree(name string) (*parse.Tree, bool) {
	ts := (*template.Template)(f).Lookup(name)
	if ts == nil {
		return nil, false
	}
	return ts.Tree, true
}
