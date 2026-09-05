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

	"github.com/typelate/muxt/internal/asteval"
)

type TemplateCallsConfiguration struct {
	TemplatesVariable string
	FilterTemplates   []*regexp.Regexp
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
func NewTemplateCalls(config TemplateCallsConfiguration, lt *asteval.LoadedTemplates) (*TemplateCalls, error) {
	global, ts := lt.Global, lt.HTML
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
	for c := range lt.Templates.ExecuteTemplateCalls() {
		t := ts.Lookup(c.TemplateName)
		if t != nil && t.Tree != nil {
			_ = check.Execute(global, t.Tree, c.DataType)
		}
	}

	var result TemplateCalls
	names := slices.Sorted(maps.Keys(refs))
	for _, name := range names {
		if len(config.FilterTemplates) > 0 && !matchesAny(name, config.FilterTemplates) {
			continue
		}
		result.Templates = append(result.Templates, NewNamedReferences(lt.Package.PkgPath, name, refs[name]))
	}

	return &result, nil
}
