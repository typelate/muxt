package analysis

import (
	"go/ast"
	"go/token"
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

type TemplateCallersConfiguration struct {
	TemplatesVariable string
	FilterTemplates   []*regexp.Regexp
}

type TemplateCallers struct {
	Templates []NamedReferences
}

// NewTemplateCallers shows where templates are referenced
func NewTemplateCallers(config TemplateCallersConfiguration, w io.Writer, fileSet *token.FileSet, pkg *packages.Package, global *check.Global, ts *template.Template) error {
	refs := make(map[string][]TemplateReference) // template name -> list of references

	// Track {{template}} calls
	global.TemplateNodeType = func(tree *parse.Tree, node *parse.TemplateNode, data types.Type) {
		pos := asteval.NewParseNodePosition(tree, node)
		refs[node.Name] = append(refs[node.Name], TemplateReference{
			Position: pos,
			Kind:     ParseTemplateNode,
			Name:     tree.Name,
			data:     data,
		})
	}

	// Find ExecuteTemplate calls
	for _, file := range pkg.Syntax {
		for node := range ast.Preorder(file) {
			templateName, dataType, ok := asteval.ExecuteTemplateArguments(node, pkg.TypesInfo, config.TemplatesVariable)
			if !ok {
				continue
			}

			refs[templateName] = append(refs[templateName], TemplateReference{
				Position: fileSet.Position(node.Pos()),
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
		result.Templates = append(result.Templates, NewNamedReferences(pkg.PkgPath, name, refs[name]))
	}
	return templates.ExecuteTemplate(w, "template_callers.txt.template", result)
}
