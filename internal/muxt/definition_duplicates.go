package muxt

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// CheckForDuplicatePatterns fails when distinct route template
// definitions produce the same normalized pattern. Templates that share
// a NAME never reach this check: html/template keeps only the last
// definition of a name, so overriding a template is allowed.
func CheckForDuplicatePatterns(templates []Definition) error {
	patterns := make(map[string][]Definition)
	order := make([]string, 0, len(templates))
	for _, def := range templates {
		pat := def.Pattern()
		if _, ok := patterns[pat]; !ok {
			order = append(order, pat)
		}
		patterns[pat] = append(patterns[pat], def)
	}
	for _, pat := range order {
		defs := patterns[pat]
		if len(defs) < 2 {
			continue
		}
		// Definitions with the same pattern tie in the byPathThenMethod
		// ordering and the template set iterates in map order, so the
		// group is sorted here to keep the report stable across runs.
		slices.SortFunc(defs, Definition.bySourceThenName)
		dup := &DuplicatePatternError{Pattern: pat}
		for _, def := range defs {
			dup.Locations = append(dup.Locations, def.definitionLocation())
		}
		return dup
	}
	return nil
}

// DuplicatePatternError reports template definitions whose names all
// produce the same route pattern. Error is the short single-line form;
// MultiLineError renders one definition location per line, path first,
// so long absolute paths stay readable and clickable in a terminal.
type DuplicatePatternError struct {
	// Pattern is the normalized http.ServeMux pattern the names produce.
	Pattern string

	// Locations renders where each definition's name literal was
	// written, in definition order: a file:line:col position, a bare
	// file name, or "" when the source is unknown.
	Locations []string
}

func (e *DuplicatePatternError) Error() string {
	return fmt.Sprintf("duplicate route pattern %q", e.Pattern)
}

// MultiLineError renders the short form followed by the location of
// every definition, one per line.
func (e *DuplicatePatternError) MultiLineError() string {
	var sb strings.Builder
	sb.WriteString(e.Error())
	note := "first defined here"
	for _, location := range e.Locations {
		if location == "" {
			continue
		}
		_, _ = fmt.Fprintf(&sb, "\n%s: %s", location, note)
		note = "also defined here"
	}
	return sb.String()
}

func (def Definition) byPathThenMethod(d Definition) int {
	if n := cmp.Compare(def.path, d.path); n != 0 {
		return n
	}
	if m := cmp.Compare(def.method, d.method); m != 0 {
		return m
	}
	return cmp.Compare(def.handler, d.handler)
}
