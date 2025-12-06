package analysis

import (
	"fmt"
	"html/template"
	"io"
)

type TemplateSourceConfiguration struct {
	TemplatesVariable string
	TemplateName      string
}

// NewTemplateSource shows the source code of templates
func NewTemplateSource(config TemplateSourceConfiguration, w io.Writer, ts *template.Template) error {
	t := ts.Lookup(config.TemplateName)
	if t == nil {
		return fmt.Errorf("template %s not found", config.TemplateName)
	}

	source := ""
	if t.Tree != nil && t.Tree.Root != nil {
		source = t.Tree.Root.String()
	}

	_, err := w.Write([]byte(source))
	return err
}
