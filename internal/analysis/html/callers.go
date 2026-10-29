package analysis

import (
	"fmt"

	"github.com/typelate/muxt/internal/template"
)

// Callers walks the AST rooted at tree and returns all TemplateNode call sites
// grouped by the template name they reference.
func Callers(tree *template.Tree) map[string][]*template.TemplateNode {
	result := make(map[string][]*template.TemplateNode)
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		if tmpl, ok := n.(*template.TemplateNode); ok {
			result[tmpl.Name] = append(result[tmpl.Name], tmpl)
		}
	})
	return result
}

// ContextConflict reports a template name that is called from multiple
// incompatible contexts.
type ContextConflict struct {
	Name     string                   // Template name.
	Contexts []*template.TemplateNode // The conflicting call sites.
}

func (c ContextConflict) Error() string {
	return fmt.Sprintf("template %q is called from conflicting contexts", c.Name)
}

// ContextConflicts examines the call sites collected by Callers and returns
// a ContextConflict for each template name that is called from call sites
// with differing HTMLContext State values. A template called from both "text"
// and "attr" context, for example, cannot be safely escaped with a single
// strategy.
func ContextConflicts(callers map[string][]*template.TemplateNode) []ContextConflict {
	var conflicts []ContextConflict
	for name, nodes := range callers {
		if len(nodes) < 2 {
			continue
		}
		first := nodes[0].HTMLContext
		if first == nil {
			continue
		}
		hasConflict := false
		for _, n := range nodes[1:] {
			if n.HTMLContext == nil {
				continue
			}
			if n.HTMLContext.State != first.State {
				hasConflict = true
				break
			}
		}
		if hasConflict {
			conflicts = append(conflicts, ContextConflict{
				Name:     name,
				Contexts: nodes,
			})
		}
	}
	return conflicts
}
