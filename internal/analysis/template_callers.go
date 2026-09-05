package analysis

import (
	"bytes"
	"go/token"
	"go/types"
	"io"
	"maps"
	"regexp"
	"slices"
	"text/template/parse"

	"github.com/typelate/check"
)

type TemplateCallersConfiguration struct {
	TemplatesVariable string
	FilterTemplates   []*regexp.Regexp
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
func NewTemplateCallers(config TemplateCallersConfiguration, fileSet *token.FileSet, lt *LoadedTemplates) (*TemplateCallers, error) {
	global, ts := lt.Global, lt.HTML
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
		for c := range lt.Templates.ExecuteTemplateCalls() {
			templateName, dataType := c.TemplateName, c.DataType

			refs[templateName] = append(refs[templateName], TemplateReference{
				Position: fileSet.Position(c.Call.Pos()),
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
		result.Templates = append(result.Templates, NewNamedReferences(lt.Package.PkgPath, name, refs[name]))
	}

	return &result, nil
}
