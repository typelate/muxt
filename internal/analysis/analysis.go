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
	"repeat":        strings.Repeat,
	"filepathBase":  filepath.Base,
	"indent":        indent,
	"formatImports": func(im *astgen.TypeFormatter) string { return strings.TrimSpace(astgen.Format(im.GenDecl())) },
}).ParseFS(outputTemplates, "*"))

// matchesAny returns true if value contains any of the filter patterns (case-insensitive substring match)
func matchesAny(value string, filters []*regexp.Regexp) bool {
	for _, filter := range filters {
		if filter.MatchString(value) {
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
		return "template"
	case ExecuteTemplateNode:
		return "execute_template"
	default:
		return "<unknown template reference kind>"
	}
}

func (k TemplateReferenceKind) MarshalText() (text []byte, err error) {
	return []byte(k.String()), nil
}

type TemplateReference struct {
	Name     string
	Kind     TemplateReferenceKind
	Position token.Position
	data     types.Type
	Data     string `json:",omitempty"`
}

type NamedReferences struct {
	Name       string
	Imports    *astgen.TypeFormatter `json:",omitempty"`
	References []TemplateReference
}

func NewNamedReferences(pkgPath, name string, refs []TemplateReference) NamedReferences {
	im := astgen.NewTypeFormatter(pkgPath)
	for i, ref := range refs {
		refs[i].Data = types.TypeString(ref.data, im.Qualifier)
	}

	slices.SortFunc(refs, func(a, b TemplateReference) int {
		c := strings.Compare(a.Name, b.Name)
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
		return strings.Compare(a.Data, b.Data)
	})
	refs = slices.CompactFunc(refs, func(a, b TemplateReference) bool {
		return a.Kind == b.Kind && a.Name == b.Name && a.Position == b.Position && a.Data == b.Data
	})

	nr := NamedReferences{
		Name:       name,
		References: refs,
	}
	if len(im.Imports) != 0 {
		nr.Imports = im
	}
	return nr
}
