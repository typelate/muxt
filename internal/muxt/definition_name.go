package muxt

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ettle/strcase"
)

func (def Definition) generateEndpointPatternIdentifier(sb *strings.Builder) string {
	if sb == nil {
		sb = new(strings.Builder)
	}
	sb.Reset()
	switch def.method {
	case http.MethodPost:
		sb.WriteString("Create")
	case http.MethodGet:
		sb.WriteString("Read")
	case http.MethodPut:
		sb.WriteString("Replace")
	case http.MethodPatch:
		sb.WriteString("Update")
	case http.MethodDelete:
		sb.WriteString("Delete")
	default:
		sb.WriteString(strcase.ToGoPascal(def.method))
	}
	var pathParams []string
	if def.path == "/" {
		if def.host != "" {
			sb.WriteString(strcase.ToGoPascal(def.host))
		}
		sb.WriteString("Index")
	} else {
		pathSegments := []string{def.host}
		pathSegments = append(pathSegments, strings.Split(def.path, "/")...)
		for _, pathSegment := range pathSegments {
			isPathParam := false
			if len(pathSegment) > 2 && pathSegment[0] == '{' && pathSegment[len(pathSegment)-1] == '}' {
				pathSegment = pathSegment[1 : len(pathSegment)-1]
				isPathParam = true
			}
			if len(pathSegment) == 0 {
				continue
			}
			if pathSegment == "$" {
				sb.WriteString("Index")
				continue
			}
			pathSegment = strings.TrimRight(pathSegment, ".")
			pathSegment = strcase.ToGoPascal(pathSegment)
			if isPathParam {
				pathParams = append(pathParams, pathSegment)
				continue
			}
			sb.WriteString(pathSegment)
		}
	}
	if len(pathParams) > 0 {
		sb.WriteString("By")
	}
	for i, pathParam := range pathParams {
		if len(pathParams) > 1 && i == len(pathParams)-1 {
			sb.WriteString("And")
		}
		sb.WriteString(pathParam)
	}
	return sb.String()
}

func calculateIdentifiers(in []Definition) {
	var (
		sb     strings.Builder
		idents = make([]string, 0, len(in))
		dupes  []string
	)
	for i, t := range in {
		if t.fun != nil && t.fun.Name != "" {
			ident := t.fun.Name
			if j := slices.Index(idents, ident); j > 0 {
				routePrev := in[j].generateEndpointPatternIdentifier(&sb)
				idents[i] = routePrev + "Calling" + ident
				route := t.generateEndpointPatternIdentifier(&sb)
				idents = append(idents, route+"Calling"+t.fun.Name)
				dupes = append(dupes, idents[j])
				in[i].identifier = ident
				continue
			}
			if slices.Contains(dupes, ident) {
				route := t.generateEndpointPatternIdentifier(&sb)
				idents = append(idents, route+"Calling"+t.fun.Name)
				in[i].identifier = ident
				continue
			}
			idents = append(idents, t.fun.Name)
			in[i].identifier = ident
			continue
		}
		ident := t.generateEndpointPatternIdentifier(&sb)
		in[i].identifier = ident
	}
}

// FileNameToPrivateIdentifier converts a template source filename to a private (unexported) Go identifier prefix.
// For example: "index.gohtml" -> "index", "user-profile.gohtml" -> "userProfile"
// Returns empty string for empty filenames.
func FileNameToPrivateIdentifier(filename string) string {
	if filename == "" {
		return ""
	}
	// Strip the extension
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if base == "" {
		return ""
	}
	// Convert to camelCase using strcase to ensure it's private (unexported)
	return strcase.ToGoCamel(base)
}
