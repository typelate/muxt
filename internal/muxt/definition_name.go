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

func (def Definition) exportedFunctionName() string {
	if def.fun == nil || def.fun.Name == "" {
		return ""
	}
	return strcase.ToGoPascal(def.fun.Name)
}

func calculateIdentifiers(in []Definition) {
	var (
		sb    strings.Builder
		dupes []string
	)
	for i, t := range in {
		if t.fun == nil || t.fun.Name == "" {
			in[i].identifier = t.generateEndpointPatternIdentifier(&sb)
			continue
		}
		ident := t.fun.Name
		exported := t.exportedFunctionName()
		if slices.Contains(dupes, ident) {
			route := t.generateEndpointPatternIdentifier(&sb)
			in[i].identifier = route + "Calling" + exported
			continue
		}
		j := slices.IndexFunc(in[:i], func(d Definition) bool {
			return d.fun != nil && d.fun.Name == ident
		})
		if j >= 0 {
			routePrev := in[j].generateEndpointPatternIdentifier(&sb)
			in[j].identifier = routePrev + "Calling" + exported
			route := t.generateEndpointPatternIdentifier(&sb)
			in[i].identifier = route + "Calling" + exported
			dupes = append(dupes, ident)
			continue
		}
		in[i].identifier = exported
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
