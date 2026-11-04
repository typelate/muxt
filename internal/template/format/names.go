package format

import (
	"go/printer"
	"maps"
	"strconv"
	"strings"
	"unicode"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/template"
)

// KeepNames sets which names keep their spellings: a template name
// whose canonical form (CanonicalName) keep reports is written as it is
// wherever it appears. muxt fmt keeps a name that a template of the
// package outside the run spells otherwise (KeptNames), so every
// reference to a template keeps naming it. Without KeepNames every name
// the file holds may be rewritten.
func KeepNames(keep func(canonical string) bool) Option {
	return func(f *formatter) {
		f.keep = keep
	}
}

// CanonicalName returns name as the formatter writes it in a
// {{define}}, {{block}} or {{template}}. A name that muxt
// reads as a route definition (muxt.ParseDefinitionName) is its method,
// pattern, status code and call, each present one, joined by single
// spaces, the call printed as gofmt prints Go (M(ctx, id)). Any other
// name, and a route name holding a line break, a comment, or spaces in
// its host, is returned as it is.
func CanonicalName(name string) string {
	if c, ok := canonicalName(name); ok {
		return c
	}
	return name
}

func canonicalName(name string) (string, bool) {
	c, ok := joinDefinitionName(name)
	if !ok {
		return "", false
	}
	// The canonical name must read as the same route and be its own
	// canonical form.
	if again, ok := joinDefinitionName(c); !ok || again != c {
		return "", false
	}
	return c, true
}

func joinDefinitionName(name string) (string, bool) {
	if strings.ContainsAny(name, "\n\r") {
		return "", false
	}
	parts, ok := muxt.ParseDefinitionName(name)
	if !ok || strings.ContainsFunc(parts.Host, unicode.IsSpace) {
		return "", false
	}
	words := make([]string, 0, 4)
	if parts.Method != "" {
		words = append(words, parts.Method)
	}
	words = append(words, parts.Host+parts.Path)
	if parts.Status != "" {
		words = append(words, parts.Status)
	}
	if parts.Call != nil {
		// The Go parser drops comments: a call holding one is as written.
		if strings.Contains(name, "/*") || strings.Contains(name, "//") {
			return "", false
		}
		var b strings.Builder
		if err := printer.Fprint(&b, parts.FileSet, parts.Call); err != nil || strings.Contains(b.String(), "\n") {
			return "", false
		}
		words = append(words, b.String())
	}
	return strings.Join(words, " "), true
}

// KeptNames returns the canonical names (CanonicalName) whose spellings
// must stay as written when the templates of a package are run, those
// formatted, and outside, those not: a name a template outside the run
// spells otherwise than canonically, so the references in the run keep
// matching it, and a name two distinct define or block names of the
// package share, which would define it twice.
func KeptNames(run, outside []*template.Tree) map[string]bool {
	kept := make(map[string]bool)
	definitions := make(map[string]string) // canonical name → a spelling defined
	visit := func(t *template.Tree, isOutside bool) {
		template.Walk(t, func(_ *template.Tree, n template.Node) {
			var name string
			definition := true
			switch n := n.(type) {
			case *template.DefineNode:
				name = n.Name
			case *template.BlockNode:
				name = n.Name
			case *template.TemplateNode:
				name, definition = n.Name, false
			default:
				return
			}
			c, ok := canonicalName(name)
			if !ok {
				c = name
			}
			if isOutside && c != name {
				kept[c] = true
			}
			if !definition {
				return
			}
			if spelled, ok := definitions[c]; ok && spelled != name {
				kept[c] = true
			}
			definitions[c] = name
		})
	}
	for _, t := range run {
		visit(t, false)
	}
	for _, t := range outside {
		visit(t, true)
	}
	return kept
}

// planRenames sets the head of each printed name token to the canonical
// name and records the renames: a name keeps its spelling
// when keep reports it, when two define or block names of root share
// its canonical form (KeptNames), or when a token naming it, or a
// spelling of it, is left as written (a muxt:fmt region, say), so every
// reference in the file changes together or none does. A {{template}}
// call is renamed only with the define or block it resolves to: one
// whose exact spelling neither root nor context defines (an execution
// error, or a template registered from Go) is as written, since its
// canonical spelling may name another template.
func (e *emitter) planRenames(root *template.ListNode, names nameRule) {
	kept := KeptNames([]*template.Tree{{Root: root}}, nil)
	defined := definedNames(root)
	for _, t := range names.context {
		maps.Copy(defined, definedNames(t.Root))
	}
	for _, name := range e.unprinted {
		if c, ok := canonicalName(name); ok && c != name {
			kept[c] = true
		}
	}
	for _, tok := range e.actions {
		if !tok.named || !tok.asWritten {
			continue
		}
		if c, ok := canonicalName(tok.name); ok && c != tok.name {
			kept[c] = true
		}
	}
	for _, tok := range e.printed {
		if !tok.named || tok.call && !defined[tok.name] {
			continue
		}
		c, ok := canonicalName(tok.name)
		if !ok || c == tok.name || kept[c] || names.keep != nil && names.keep(c) {
			continue
		}
		if e.renames == nil {
			e.renames = make(map[string]string)
		}
		e.renames[tok.name] = c
		tok.head = strings.TrimSuffix(tok.head, tok.literal) + quoteLike(tok.literal, c)
	}
}

// nameRule is what planRenames renames names by: the names KeepNames
// keeps, and the package's other templates, whose defines and blocks
// {{template}} calls may resolve to.
type nameRule struct {
	keep    func(canonical string) bool
	context []*template.Tree
}

// renamed returns the rename actionShapes reads a source's names with,
// given the renames the emitter made.
func renamed(renames map[string]string) func(string) string {
	return func(name string) string {
		if c, ok := renames[name]; ok {
			return c
		}
		return name
	}
}

// quoteLike returns s quoted as literal is: in backticks when literal
// is a raw string and s can be one, else as a Go string literal.
func quoteLike(literal, s string) string {
	if strings.HasPrefix(literal, "`") && !strings.Contains(s, "`") {
		return "`" + s + "`"
	}
	return strconv.Quote(s)
}
