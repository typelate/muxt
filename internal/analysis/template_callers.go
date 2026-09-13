package analysis

import (
	"bytes"
	"go/types"
	"io"
	"maps"
	"regexp"
	"slices"
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
		result, err := templateCallers(config, pkg, lt)
		if err != nil {
			return nil, err
		}
		combined.Templates = append(combined.Templates, result.Templates...)
	}
	return combined, nil
}

func templateCallers(config TemplateCallersConfiguration, pkg source.Package, lt source.Variable) (*TemplateCallers, error) {
	global, ts := newGlobal(pkg, lt), lt.Set
	refs := make(map[string][]TemplateReference) // template name -> list of references

	// Track {{template}} calls
	global.InspectTemplateNode = func(node *parse.TemplateNode, tree *parse.Tree, data types.Type, _ check.Definition) {
		pos := check.ParseNodePosition(tree, node)
		refs[node.Name] = append(refs[node.Name], TemplateReference{
			Position: pos,
			Kind:     ParseTemplateNode,
			Name:     tree.Name,
			data:     data,
		})
	}

	{
		for _, c := range lt.Calls {
			templateName, dataType := c.Template, c.Data

			refs[templateName] = append(refs[templateName], TemplateReference{
				Position: c.Position,
				Kind:     ExecuteTemplateNode,
				Name:     templateName,
				data:     dataType,
			})

			// Analyze the template to find {{template}} calls
			t := ts.Lookup(templateName)
			if t != nil && t.Tree != nil {
				_ = check.Execute(global, t.Tree, dataType)
			}
		}
	}

	var result TemplateCallers
	names := slices.Sorted(maps.Keys(refs))
	for _, name := range names {
		if len(config.FilterTemplates) > 0 && !matchesAny(name, config.FilterTemplates) {
			continue
		}
		result.Templates = append(result.Templates, NewNamedReferences(pkg.Types.Path(), name, refs[name]))
	}

	return &result, nil
}
