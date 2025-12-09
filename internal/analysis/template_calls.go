package analysis

import (
	"bytes"
	"go/ast"
	"go/types"
	"html/template"
	"io"
	"maps"
	"regexp"
	"slices"
	"text/template/parse"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"

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
func NewTemplateCalls(config TemplateCallsConfiguration, pkg *packages.Package, global *check.Global, ts *template.Template) (*TemplateCalls, error) {
	// Track what each template uses (calls via {{template}})
	refs := make(map[string][]TemplateReference) // template -> set of templates it calls

	global.TemplateNodeType = func(tree *parse.Tree, node *parse.TemplateNode, data types.Type) {
		refs[tree.Name] = append(refs[tree.Name], TemplateReference{
			Name:     node.Name,
			Kind:     ParseTemplateNode,
			Position: asteval.NewParseNodePosition(tree, node),
			data:     data,
		})
	}

	// Analyze all templates
	for _, file := range pkg.Syntax {
		for node := range ast.Preorder(file) {
			templateName, dataType, ok := asteval.ExecuteTemplateArguments(node, pkg.TypesInfo, config.TemplatesVariable)
			if !ok {
				continue
			}
			t := ts.Lookup(templateName)
			if t != nil && t.Tree != nil {
				_ = check.Execute(global, t.Tree, dataType)
			}
		}
	}

	var result TemplateCalls
	names := slices.Sorted(maps.Keys(refs))
	for _, name := range names {
		if len(config.FilterTemplates) > 0 && !matchesAny(name, config.FilterTemplates) {
			continue
		}
		result.Templates = append(result.Templates, NewNamedReferences(pkg.PkgPath, name, refs[name]))
	}
	return &result, nil
}
