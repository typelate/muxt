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

type TemplateCallsConfiguration struct {
	// TemplatesVariables are listed in order.
	TemplatesVariables []string

	// FilterTemplates, when set, limits the listing to templates whose
	// name matches one of them.
	FilterTemplates []*regexp.Regexp
}

type TemplateCalls struct {
	Templates []NamedReferences
}

func (result *TemplateCalls) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	err := templates.ExecuteTemplate(&buf, "template_calls.txt.template", result)
	if err != nil {
		return 0, err
	}
	return io.Copy(w, &buf)
}

// NewTemplateCalls shows what templates use (other templates they call)
func NewTemplateCalls(config TemplateCallsConfiguration, pkg source.Package) (*TemplateCalls, error) {
	combined := &TemplateCalls{}
	for _, lt := range pkg.Variables {
		combined.Templates = append(combined.Templates, templateCalls(config, pkg, lt)...)
	}
	return combined, nil
}

func templateCalls(config TemplateCallsConfiguration, pkg source.Package, lt source.Variable) []NamedReferences {
	global := newGlobal(pkg, lt)
	refs := make(map[string][]TemplateReference) // template -> set of templates it calls

	global.InspectTemplateNode = func(node *parse.TemplateNode, tree *parse.Tree, data types.Type, _ check.Definition) {
		refs[tree.Name] = append(refs[tree.Name], TemplateReference{
			Name:     node.Name,
			Kind:     ParseTemplateNode,
			Position: check.ParseNodePosition(tree, node),
			data:     data,
		})
	}

	for _, c := range lt.Calls {
		executeTemplateTree(global, lt.Set, c.Template, c.Data)
	}

	return newReferences(pkg.Types.Path(), refs, config.FilterTemplates)
}
