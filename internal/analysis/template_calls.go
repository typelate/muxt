package analysis

import (
	"go/ast"
	"go/types"
	"html/template"
	"io"
	"maps"
	"slices"
	"text/template/parse"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
)

type TemplateCallsConfiguration struct {
	TemplatesVariable string
	FilterTemplates   []string
}

type TemplateCalls struct {
	Templates []NamedReferences
}

// NewTemplateCalls shows what templates use (other templates they call)
func NewTemplateCalls(config TemplateCallsConfiguration, w io.Writer, pkg *packages.Package, global *check.Global, ts *template.Template) error {
	// Track what each template uses (calls via {{template}})
	uses := make(map[string][]TemplateReference) // template -> set of templates it calls

	global.TemplateNodeType = func(currentTree *parse.Tree, node *parse.TemplateNode, data types.Type) {
		uses[currentTree.Name] = append(uses[currentTree.Name], TemplateReference{
			Name:     node.Name,
			Kind:     ParseTemplateNode,
			Position: asteval.NewParseNodePosition(currentTree, node),
			Data:     data,
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
	names := slices.Sorted(maps.Keys(uses))
	for _, name := range names {
		if len(config.FilterTemplates) > 0 && !matchesAny(name, config.FilterTemplates) {
			continue
		}
		result.Templates = append(result.Templates, NamedReferences{
			Name:       name,
			References: uses[name],
		})
	}
	return templates.ExecuteTemplate(w, "template_calls.txt.template", result)
}
