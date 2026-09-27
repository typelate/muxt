package muxt

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/source"
)

// SegmentKind classifies one "/"-separated part of a route pattern's path.
type SegmentKind int

const (
	SegmentKindUnknown SegmentKind = iota
	// SegmentKindLiteral matches its text exactly.
	SegmentKindLiteral
	// SegmentKindWildcard is a {name} segment: it matches one path segment
	// and names a path parameter.
	SegmentKindWildcard
	// SegmentKindWildcardRemainder is a trailing {name...} segment: it
	// matches the rest of the path and names a path parameter.
	SegmentKindWildcardRemainder
)

// Segment is one "/"-separated part of a route pattern's path, after any
// trailing {$}. A wildcard segment names a path parameter; once ResolveCall
// has run it also knows the type the parameter parses into.
type Segment struct {
	kind  SegmentKind
	value string

	tp            types.Type
	textMarshaler bool
}

// initializeSegments splits the path into segments and checks each wildcard
// names a distinct, unreserved Go identifier.
func (def *Definition) initializeSegments() error {
	templatePath := strings.TrimSuffix(def.path, "{$}")
	parts := strings.Split(templatePath, "/")[1:]
	segments := make([]Segment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		segment := newSegment(part)
		if !segment.IsLiteral() {
			if err := def.checkPathParameterName(segment, segments); err != nil {
				return err
			}
		}
		segments = append(segments, segment)
	}
	def.Segments = segments
	return nil
}

func (def *Definition) checkPathParameterName(segment Segment, before []Segment) error {
	n := segment.value
	if segment.kind == SegmentKindUnknown {
		return def.pathParamErrorf(n, 0, "path segment {%s is not permitted: a wildcard is spelled {name} or {name...}", n)
	}
	if !token.IsIdentifier(n) {
		return def.pathParamErrorf(n, 0, "path parameter name not permitted: %q is not a Go identifier", n)
	}
	if _, dup := pathParameter(before, n); dup {
		return def.pathParamErrorf(n, 1, "path parameter name %q is used more than once; parameter names must be unique within a path", n)
	}
	if slices.Contains(patternScope(), n) {
		return def.pathParamErrorf(n, 0, "path parameter name %s conflicts with a reserved identifier (%s)", n, strings.Join(patternScope(), ", "))
	}
	return nil
}

func newSegment(in string) Segment {
	inner, ok := strings.CutPrefix(in, "{")
	if !ok {
		return Segment{value: in, kind: SegmentKindLiteral}
	}
	if value, ok := strings.CutSuffix(inner, "...}"); ok {
		return Segment{value: value, kind: SegmentKindWildcardRemainder}
	}
	if value, ok := strings.CutSuffix(inner, "}"); ok {
		return Segment{value: value, kind: SegmentKindWildcard}
	}
	return Segment{value: inner, kind: SegmentKindUnknown}
}

// pathParameter returns the wildcard segment that names the path parameter.
func pathParameter(segments []Segment, name string) (Segment, bool) {
	for _, segment := range segments {
		if segment.IsWildcard() && segment.value == name {
			return segment, true
		}
	}
	return Segment{}, false
}

// Value is the text of a literal segment or the parameter name of a
// wildcard segment.
func (s Segment) Value() string { return s.value }

func (s Segment) Kind() SegmentKind { return s.kind }
func (s Segment) IsLiteral() bool   { return s.kind == SegmentKindLiteral }

// IsWildcard reports whether the segment names a path parameter, as either
// {name} or {name...}.
func (s Segment) IsWildcard() bool {
	return s.kind == SegmentKindWildcard || s.kind == SegmentKindWildcardRemainder
}

// IsRemainder reports whether the segment is a {name...} wildcard, whose
// value is the rest of the path.
func (s Segment) IsRemainder() bool { return s.kind == SegmentKindWildcardRemainder }

// Type returns the type a wildcard segment's value parses into: the
// parameter type where the call first passes it, or string when the value
// is passed along as it arrived or is not passed to the call at all.
func (s Segment) Type() source.Type {
	if s.tp == nil {
		return source.NewType(types.Universe.Lookup("string").Type())
	}
	return source.NewType(s.tp)
}

// TextMarshaler reports whether Type implements encoding.TextMarshaler, so
// a route path formats the value with MarshalText.
func (s Segment) TextMarshaler() bool { return s.textMarshaler }
