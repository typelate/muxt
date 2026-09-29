package muxt

import (
	"fmt"
	"go/token"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ettle/strcase"
)

func (def Definition) generateEndpointPatternIdentifier(sb *strings.Builder) string {
	if sb == nil {
		sb = new(strings.Builder)
	}
	sb.Reset()
	sb.WriteString(methodVerb(def.method))
	var pathParams []string
	if def.path == "/" {
		sb.WriteString(strcase.ToGoPascal(def.host))
		sb.WriteString("Index")
	} else {
		pathParams = writePathSegments(sb, append([]string{def.host}, strings.Split(def.path, "/")...))
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

func methodVerb(method string) string {
	switch method {
	case http.MethodPost:
		return "Create"
	case http.MethodGet:
		return "Read"
	case http.MethodPut:
		return "Replace"
	case http.MethodPatch:
		return "Update"
	case http.MethodDelete:
		return "Delete"
	}
	return strcase.ToGoPascal(method)
}

// writePathSegments writes the literal segments to sb and returns the names of
// the wildcard segments, which are written after them.
func writePathSegments(sb *strings.Builder, segments []string) []string {
	var pathParams []string
	for _, segment := range segments {
		name, isPathParam := wildcardName(segment)
		switch {
		case name == "":
			continue
		case isPathParam && name == "$":
			sb.WriteString("Exact")
		case isPathParam:
			pathParams = append(pathParams, strcase.ToGoPascal(strings.TrimRight(name, ".")))
		default:
			sb.WriteString(strcase.ToGoPascal(strings.TrimRight(name, ".")))
		}
	}
	return pathParams
}

func wildcardName(segment string) (string, bool) {
	if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
		return segment[1 : len(segment)-1], true
	}
	return segment, false
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

// ExportedPathIdentifier returns the definition's identifier with its first
// rune uppercased: the TemplateRoutePaths method name for this route.
func (def Definition) ExportedPathIdentifier() (string, error) {
	return exportPathIdentifier(def.identifier)
}

func exportPathIdentifier(s string) (string, error) {
	r, size := utf8.DecodeRuneInString(s)
	exported := string(utf8.AppendRune(nil, unicode.ToUpper(r))) + s[size:]
	if !token.IsExported(exported) {
		return "", fmt.Errorf("cannot export identifier %q for TemplateRoutePaths method: first character %q has no uppercase form", s, r)
	}
	return exported, nil
}

// CheckPathMethodCollisions rejects definition sets in which two handlers
// produce the same TemplateRoutePaths method name (e.g. "list" and "List").
// Duplicate route patterns also collide here, so check
// CheckForDuplicatePatterns first for the more precise error.
func CheckPathMethodCollisions(defs []Definition) error {
	seen := make(map[string]Definition, len(defs))
	handlerName := func(def Definition) string {
		// Report the original handler names (e.g. "list" and "List"), not the
		// exported identifier they collide on, so the difference is visible.
		if handler := def.functionName(); handler != "" {
			return handler
		}
		return def.Identifier()
	}
	for _, t := range defs {
		exported, err := exportPathIdentifier(t.Identifier())
		if err != nil {
			return err
		}
		if prev, ok := seen[exported]; ok {
			return &MethodNameCollisionError{
				Method:    exported,
				Handlers:  [2]string{handlerName(prev), handlerName(t)},
				Locations: [2]string{prev.definitionLocation(), t.definitionLocation()},
			}
		}
		seen[exported] = t
	}
	return nil
}

// MethodNameCollisionError reports two handlers whose names produce the
// same exported TemplateRoutePaths method. Error is the short
// single-line form; MultiLineError adds one definition location per
// line.
type MethodNameCollisionError struct {
	// Method is the exported method name both handlers produce.
	Method string

	// Handlers are the original handler names, in definition order.
	Handlers [2]string

	// Locations render where each definition's name literal was
	// written; "" when the source is unknown.
	Locations [2]string
}

func (e *MethodNameCollisionError) Error() string {
	return fmt.Sprintf("TemplateRoutePaths method name collision: handlers %q and %q both produce method %q", e.Handlers[0], e.Handlers[1], e.Method)
}

// MultiLineError renders the short form followed by where each
// colliding handler is defined, one location per line.
func (e *MethodNameCollisionError) MultiLineError() string {
	var sb strings.Builder
	sb.WriteString(e.Error())
	for i, location := range e.Locations {
		if location == "" {
			continue
		}
		fmt.Fprintf(&sb, "\n%s: %q is defined here", location, e.Handlers[i])
	}
	return sb.String()
}

// FileNameToPrivateIdentifier converts a template source filename to a private (unexported) Go identifier prefix.
// For example: "index.gohtml" -> "index", "user-profile.gohtml" -> "userProfile"
func FileNameToPrivateIdentifier(filename string) string {
	if filename == "" {
		return ""
	}
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if base == "" {
		return ""
	}
	// Convert to camelCase using strcase to ensure it's private (unexported)
	return strcase.ToGoCamel(base)
}
