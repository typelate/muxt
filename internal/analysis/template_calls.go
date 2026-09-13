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
		result, err := templateCalls(config, pkg, lt)
		if err != nil {
			return nil, err
		}
		combined.Templates = append(combined.Templates, result.Templates...)
	}
	return combined, nil
}

func templateCalls(config TemplateCallsConfiguration, pkg source.Package, lt source.Variable) (*TemplateCalls, error) {
	global, ts := newGlobal(pkg, lt), lt.Set
	// Track what each template uses (calls via {{template}})
	refs := make(map[string][]TemplateReference) // template -> set of templates it calls

	global.InspectTemplateNode = func(node *parse.TemplateNode, tree *parse.Tree, data types.Type, _ check.Definition) {
		refs[tree.Name] = append(refs[tree.Name], TemplateReference{
			Name:     node.Name,
			Kind:     ParseTemplateNode,
			Position: check.ParseNodePosition(tree, node),
			data:     data,
		})
	}

	// Analyze all templates
	for _, c := range lt.Calls {
		t := ts.Lookup(c.Template)
		if t != nil && t.Tree != nil {
			_ = check.Execute(global, t.Tree, c.Data)
		}
	}

	var result TemplateCalls
	names := slices.Sorted(maps.Keys(refs))
	for _, name := range names {
		if len(config.FilterTemplates) > 0 && !matchesAny(name, config.FilterTemplates) {
			continue
		}
		result.Templates = append(result.Templates, NewNamedReferences(pkg.Types.Path(), name, refs[name]))
	}

	return &result, nil
}
