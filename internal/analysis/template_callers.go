package analysis

import (
	"bytes"
	"go/types"
	"io"
	"regexp"
	"text/template/parse"

	"github.com/typelate/check"

	"github.com/typelate/muxt/internal/source"
)

type TemplateCallersConfiguration struct {
	// TemplatesVariables are listed in order.
	TemplatesVariables []string

	// FilterTemplates, when set, limits the listing to templates whose
	// name matches one of them.
	FilterTemplates []*regexp.Regexp
}

type TemplateCallers struct {
	Templates []NamedReferences
}

func (result *TemplateCallers) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "template_callers.txt.template", result)
	if err != nil {
		return 0, err
	}
	return io.Copy(w, &buf)
}

// NewTemplateCallers shows where templates are referenced
func NewTemplateCallers(config TemplateCallersConfiguration, pkg source.Package) (*TemplateCallers, error) {
	combined := &TemplateCallers{}
	for _, lt := range pkg.Variables {
		combined.Templates = append(combined.Templates, templateCallers(config, pkg, lt)...)
	}
	return combined, nil
}

func templateCallers(config TemplateCallersConfiguration, pkg source.Package, lt source.Variable) []NamedReferences {
	global := newGlobal(pkg, lt)
	refs := make(map[string][]TemplateReference) // template name -> list of references

	global.InspectTemplateNode = func(node *parse.TemplateNode, tree *parse.Tree, data types.Type, _ check.Definition) {
		pos := check.ParseNodePosition(tree, node)
		refs[node.Name] = append(refs[node.Name], TemplateReference{
			Position: pos,
			Kind:     ParseTemplateNode,
			Name:     tree.Name,
			data:     data,
		})
	}

	for _, c := range lt.Calls {
		refs[c.Template] = append(refs[c.Template], TemplateReference{
			Position: c.Position,
			Kind:     ExecuteTemplateNode,
			Name:     c.Template,
			data:     c.Data,
		})
		executeTemplateTree(global, lt.Set, c.Template, c.Data)
	}

	return newReferences(pkg.Types.Path(), refs, config.FilterTemplates)
}
