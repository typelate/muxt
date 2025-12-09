package analysis

import (
	"embed"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed *.txt.template
var outputTemplates embed.FS

var templates = template.Must(template.New("output").Funcs(template.FuncMap{
	"repeat":       strings.Repeat,
	"filepathBase": filepath.Base,
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
	Data     types.Type
}

type NamedReferences struct {
	Name       string
	References []TemplateReference
}
