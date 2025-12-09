package analysis

import (
	"cmp"
	"embed"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/typelate/muxt/internal/astgen"
)

//go:embed *.txt.template
var outputTemplates embed.FS

var templates = template.Must(template.New("output").Funcs(template.FuncMap{
	"repeat":       strings.Repeat,
	"filepathBase": filepath.Base,
	"indent":       indent,
}).ParseFS(outputTemplates, "*"))

// matchesAny returns true if value contains any of the filter patterns (case-insensitive substring match)
func matchesAny(value string, filters []*regexp.Regexp) bool {
	valueLower := strings.ToLower(value)
	for _, filter := range filters {
		if filter.MatchString(valueLower) {
			return true
		}
	}
	return false
}

func indent(prefix, in string) string {
	lines := strings.Split(in, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

type TemplateReferenceKind int

const (
	ParseTemplateNode TemplateReferenceKind = 1 + iota
	ExecuteTemplateNode
)

func (k TemplateReferenceKind) String() string {
	switch k {
	case ParseTemplateNode:
		return "parse.TemplateNode"
	case ExecuteTemplateNode:
		return "template.ExecuteTemplate"
	default:
		return "<unknown template reference kind>"
	}
}

type TemplateReference struct {
	Name     string
	Kind     TemplateReferenceKind
	Position token.Position
	data     types.Type
	Data     string
}

type NamedReferences struct {
	Name       string
	References []TemplateReference
	Imports    string
}

func NewNamedReferences(pkgPath, name string, refs []TemplateReference) NamedReferences {
	im := astgen.NewTypeFormatter(pkgPath)
	for i, ref := range refs {
		refs[i].Data = types.TypeString(ref.data, im.Qualifier)
	}

	slices.SortFunc(refs, func(a, b TemplateReference) int {
		c := strings.Compare(a.Name, a.Name)
		if c != 0 {
			return c
		}
		c = cmp.Compare(a.Position.Filename, b.Position.Filename)
		if c != 0 {
			return c
		}
		c = cmp.Compare(a.Position.Offset, b.Position.Offset)
		if c != 0 {
			return c
		}
		c = strings.Compare(a.Kind.String(), b.Kind.String())
		if c != 0 {
			return c
		}
		return strings.Compare(a.Data, a.Data)
	})
	refs = slices.CompactFunc(refs, func(a, b TemplateReference) bool {
		return a.Kind == b.Kind && a.Name == b.Name && a.Position == b.Position && a.Data == b.Data
	})

	nr := NamedReferences{
		Name:       name,
		References: refs,
	}
	if len(im.Imports) != 0 {
		nr.Imports = strings.TrimSpace(astgen.Format(im.GenDecl()))
	}
	return nr
}
