package analysis

import (
	"strings"

	"github.com/typelate/muxt/internal/template"
)

// Formatting-marker boundaries (WHATWG §13.2.4.3): the active
// formatting list is cleared entering these, so an outer <a>/<button>
// does not constrain content inside them.
var markerBoundaries = map[string]bool{
	"applet": true, "object": true, "marquee": true, "template": true,
	"td": true, "th": true, "caption": true,
}

// checkNesting reports <form> elements nested inside another <form>
// (§13.2.6.4.7: the inner start tag is ignored by browsers unless a
// <template> boundary separates them) and <a>/<button> elements
// nested inside another of the same kind (browsers force-close the
// outer element when the inner one starts).
func checkNesting(t *template.Tree, diags *Diagnostics) {
	template.WalkPath(t, func(_ *template.Tree, path []template.Node, n template.Node) {
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			return
		}
		tag := strings.ToLower(el.TagName)
		switch tag {
		case "form":
			if _, mod, found := findAncestor(path, "form", map[string]bool{"template": true}); found {
				sev := template.SeverityError
				if mod != ModalityAlways {
					sev = template.SeverityWarning
				}
				appendDiagSev(diags, el.Start(), el.End(), template.DiagNestedForm, sev, mod, msgNestedForm)
			}
		case "a":
			if _, mod, found := findAncestor(path, "a", markerBoundaries); found {
				appendDiag(diags, el.Start(), el.End(), template.DiagNestedLink, mod, msgNestedLink)
			}
		case "button":
			if _, mod, found := findAncestor(path, "button", markerBoundaries); found {
				appendDiag(diags, el.Start(), el.End(), template.DiagNestedButton, mod, msgNestedButton)
			}
		}
	})
}

// findAncestor scans element ancestors nearest-first for tag,
// stopping (not found) at any boundary element.
func findAncestor(path []template.Node, tag string, boundaries map[string]bool) (*template.HTMLElementNode, Modality, bool) {
	for el, mod := range elementAncestors(path) {
		t := strings.ToLower(el.TagName)
		if t == tag {
			return el, mod, true
		}
		if boundaries[t] {
			return nil, mod, false
		}
	}
	return nil, ModalityAlways, false
}
