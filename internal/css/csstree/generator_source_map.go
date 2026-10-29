// Ported from lib/generator/sourceMap.js, with the part of source-map-js's
// SourceMapGenerator (lib/source-map-generator.js, mapping-list.js,
// base64-vlq.js, base64.js and util.js) it uses.

package csstree

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

func isTrackNode(nodeType string) bool {
	switch nodeType {
	case "Atrule", "Selector", "Declaration":
		return true
	}
	return false
}

// generateSourceMap decorates handlers to record in sourceMap where the
// output of Atrule, Selector and Declaration nodes comes from (upstream
// returns handlers whose result is { css, map }; here the map is
// sourceMap and the result stays the CSS).
func generateSourceMap(handlers GenerateHandlers, sourceMap *SourceMapGenerator) GenerateHandlers {
	generated := SourceMapPosition{
		Line:   1,
		Column: 0,
	}
	original := SourceMapPosition{
		Line:   0, // should be zero to add first mapping
		Column: 0,
	}
	activatedGenerated := SourceMapPosition{
		Line:   1,
		Column: 0,
	}
	line := 1
	column := 0
	sourceMappingActive := false

	addMapping := func(mapping SourceMapping) {
		if err := sourceMap.AddMapping(mapping); err != nil {
			panic(generateFailure{err})
		}
	}

	origHandlersNode := handlers.Node
	handlers.Node = func(h *GenerateHandlers, node Node) {
		if node != nil && node.Location() != nil && isTrackNode(node.Type()) {
			loc := node.Location()
			nodeLine := loc.Start.Line
			nodeColumn := loc.Start.Column - 1

			if original.Line != nodeLine ||
				original.Column != nodeColumn {
				original.Line = nodeLine
				original.Column = nodeColumn

				generated.Line = line
				generated.Column = column

				if sourceMappingActive {
					sourceMappingActive = false
					if generated.Line != activatedGenerated.Line ||
						generated.Column != activatedGenerated.Column {
						addMapping(SourceMapping{Generated: activatedGenerated})
					}
				}

				sourceMappingActive = true
				addMapping(SourceMapping{
					Source:    loc.Source,
					Original:  &original,
					Generated: generated,
				})
			}
		}

		origHandlersNode(h, node)

		if sourceMappingActive && node != nil && isTrackNode(node.Type()) {
			activatedGenerated.Line = line
			activatedGenerated.Column = column
		}
	}

	origHandlersEmit := handlers.Emit
	handlers.Emit = func(h *GenerateHandlers, value string, tokenType tokenizer.TokenType, auto bool) {
		for i := 0; i < len(value); i++ {
			if value[i] == '\n' {
				line++
				column = 0
			} else {
				column++
			}
		}

		origHandlersEmit(h, value, tokenType, auto)
	}

	origHandlersResult := handlers.Result
	handlers.Result = func(h *GenerateHandlers) string {
		if sourceMappingActive {
			addMapping(SourceMapping{Generated: activatedGenerated})
		}

		return origHandlersResult(h)
	}

	return handlers
}

// SourceMapPosition is a position in a source map: a 1-based line and a
// 0-based column.
type SourceMapPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// SourceMapping maps a position of the generated text to a position of a
// source (upstream addMapping's { generated, original, source, name }).
// Original nil, Source "" and Name "" are upstream null.
type SourceMapping struct {
	Generated SourceMapPosition
	Original  *SourceMapPosition
	Source    string
	Name      string
}

// mapping is a stored SourceMapping (upstream { generatedLine,
// generatedColumn, originalLine, originalColumn, source, name }).
type mapping struct {
	generatedLine   int
	generatedColumn int
	originalLine    int
	originalColumn  int
	source          string
	name            string
}

// SourceMapGenerator builds a Source Map revision 3 (the part of
// source-map-js's SourceMapGenerator that generation needs). The zero
// value is an empty source map ready to use.
type SourceMapGenerator struct {
	// File is the name of the generated file ("" omits it).
	File string

	sources         arraySet
	names           arraySet
	mappings        mappingList
	sourcesContents map[string]string
}

// AddMapping adds a mapping. A mapping has a generated position only, or
// also an original position and a source; anything else is an error
// ("Invalid mapping: ...").
func (g *SourceMapGenerator) AddMapping(m SourceMapping) error {
	if err := validateMapping(m); err != nil {
		return err
	}

	if m.Source != "" {
		g.sources.add(m.Source)
	}

	if m.Name != "" {
		g.names.add(m.Name)
	}

	stored := mapping{
		generatedLine:   m.Generated.Line,
		generatedColumn: m.Generated.Column,
		source:          m.Source,
		name:            m.Name,
	}
	if m.Original != nil {
		stored.originalLine = m.Original.Line
		stored.originalColumn = m.Original.Column
	}
	g.mappings.add(stored)

	return nil
}

func validateMapping(m SourceMapping) error {
	generatedValid := m.Generated.Line > 0 && m.Generated.Column >= 0

	if generatedValid && m.Original == nil && m.Source == "" && m.Name == "" {
		// Case 1.
		return nil
	} else if generatedValid && m.Original != nil &&
		m.Original.Line > 0 && m.Original.Column >= 0 &&
		m.Source != "" {
		// Cases 2 and 3.
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, `Invalid mapping: {"generated":{"line":%d,"column":%d},"source":`, m.Generated.Line, m.Generated.Column)
	writeNullableJSONString(&b, m.Source)
	b.WriteString(`,"original":`)
	if m.Original != nil {
		fmt.Fprintf(&b, `{"line":%d,"column":%d}`, m.Original.Line, m.Original.Column)
	} else {
		b.WriteString("null")
	}
	b.WriteString(`,"name":`)
	writeNullableJSONString(&b, m.Name)
	b.WriteString("}")
	return errors.New(b.String())
}

// SetSourceContent sets the content of a source file, which the map then
// includes as sourcesContent.
func (g *SourceMapGenerator) SetSourceContent(source, content string) {
	if g.sourcesContents == nil {
		g.sourcesContents = make(map[string]string)
	}
	g.sourcesContents[source] = content
}

func (g *SourceMapGenerator) serializeMappings() string {
	previousGeneratedColumn := 0
	previousGeneratedLine := 1
	previousOriginalColumn := 0
	previousOriginalLine := 0
	previousName := 0
	previousSource := 0
	var result strings.Builder

	mappings := g.mappings.toArray()
	for i, mapping := range mappings {
		var next strings.Builder

		if mapping.generatedLine != previousGeneratedLine {
			previousGeneratedColumn = 0
			for mapping.generatedLine != previousGeneratedLine {
				next.WriteByte(';')
				previousGeneratedLine++
			}
		} else {
			if i > 0 {
				if compareByGeneratedPositionsInflated(mapping, mappings[i-1]) == 0 {
					continue
				}
				next.WriteByte(',')
			}
		}

		base64VLQEncode(&next, mapping.generatedColumn-previousGeneratedColumn)
		previousGeneratedColumn = mapping.generatedColumn

		if mapping.source != "" {
			sourceIdx := g.sources.indexOf(mapping.source)
			base64VLQEncode(&next, sourceIdx-previousSource)
			previousSource = sourceIdx

			// lines are stored 0-based in SourceMap spec version 3
			base64VLQEncode(&next, mapping.originalLine-1-previousOriginalLine)
			previousOriginalLine = mapping.originalLine - 1

			base64VLQEncode(&next, mapping.originalColumn-previousOriginalColumn)
			previousOriginalColumn = mapping.originalColumn

			if mapping.name != "" {
				nameIdx := g.names.indexOf(mapping.name)
				base64VLQEncode(&next, nameIdx-previousName)
				previousName = nameIdx
			}
		}

		result.WriteString(next.String())
	}

	return result.String()
}

// String returns the source map as JSON (upstream toString, which is
// JSON.stringify(toJSON())): version, sources, names, mappings, then file
// and sourcesContent when set.
func (g *SourceMapGenerator) String() string {
	var b strings.Builder
	b.WriteString(`{"version":3,"sources":`)
	writeJSONStrings(&b, g.sources.toArray())
	b.WriteString(`,"names":`)
	writeJSONStrings(&b, g.names.toArray())
	b.WriteString(`,"mappings":`)
	writeJSONString(&b, g.serializeMappings())
	if g.File != "" {
		b.WriteString(`,"file":`)
		writeJSONString(&b, g.File)
	}
	if g.sourcesContents != nil {
		b.WriteString(`,"sourcesContent":[`)
		for i, source := range g.sources.toArray() {
			if i > 0 {
				b.WriteByte(',')
			}
			if content, ok := g.sourcesContents[source]; ok {
				writeJSONString(&b, content)
			} else {
				b.WriteString("null")
			}
		}
		b.WriteByte(']')
	}
	b.WriteByte('}')
	return b.String()
}

// MarshalJSON encodes the source map (upstream toJSON).
func (g *SourceMapGenerator) MarshalJSON() ([]byte, error) {
	return []byte(g.String()), nil
}

// arraySet is an insertion-ordered set of strings (source-map-js
// ArraySet).
type arraySet struct {
	array []string
	set   map[string]int
}

func (s *arraySet) add(str string) {
	if _, ok := s.set[str]; ok {
		return
	}
	if s.set == nil {
		s.set = make(map[string]int)
	}
	s.set[str] = len(s.array)
	s.array = append(s.array, str)
}

func (s *arraySet) indexOf(str string) int {
	if idx, ok := s.set[str]; ok {
		return idx
	}
	return -1
}

func (s *arraySet) toArray() []string {
	return slices.Clone(s.array)
}

// mappingList keeps mappings and sorts them by generated position only
// when they were added out of order (source-map-js MappingList).
type mappingList struct {
	array   []mapping
	sorted  bool
	hasLast bool
	last    mapping
}

// generatedPositionAfter reports whether mappingB is at or after mappingA.
func generatedPositionAfter(mappingA, mappingB mapping) bool {
	// Optimized for most common case
	lineA := mappingA.generatedLine
	lineB := mappingB.generatedLine
	columnA := mappingA.generatedColumn
	columnB := mappingB.generatedColumn
	return lineB > lineA || lineB == lineA && columnB >= columnA ||
		compareByGeneratedPositionsInflated(mappingA, mappingB) <= 0
}

func (l *mappingList) add(m mapping) {
	if len(l.array) == 0 {
		l.sorted = true
	}
	// the initial last mapping is upstream's infimum { generatedLine: -1 }
	if !l.hasLast || generatedPositionAfter(l.last, m) {
		l.hasLast = true
		l.last = m
		l.array = append(l.array, m)
	} else {
		l.sorted = false
		l.array = append(l.array, m)
	}
}

func (l *mappingList) toArray() []mapping {
	if !l.sorted {
		slices.SortStableFunc(l.array, compareByGeneratedPositionsInflated)
		l.sorted = true
	}
	return l.array
}

func compareByGeneratedPositionsInflated(mappingA, mappingB mapping) int {
	if c := cmp.Compare(mappingA.generatedLine, mappingB.generatedLine); c != 0 {
		return c
	}

	if c := cmp.Compare(mappingA.generatedColumn, mappingB.generatedColumn); c != 0 {
		return c
	}

	if c := strcmp(mappingA.source, mappingB.source); c != 0 {
		return c
	}

	if c := cmp.Compare(mappingA.originalLine, mappingB.originalLine); c != 0 {
		return c
	}

	if c := cmp.Compare(mappingA.originalColumn, mappingB.originalColumn); c != 0 {
		return c
	}

	return strcmp(mappingA.name, mappingB.name)
}

// strcmp orders strings with "" (upstream null) after every other string.
func strcmp(aStr1, aStr2 string) int {
	if aStr1 == aStr2 {
		return 0
	}

	if aStr1 == "" {
		return 1 // aStr2 != null
	}

	if aStr2 == "" {
		return -1 // aStr1 != null
	}

	return strings.Compare(aStr1, aStr2)
}

// A single base 64 digit can contain 6 bits of data. For the base 64 variable
// length quantities we use in the source map spec, the first bit is the sign,
// the next four bits are the actual value, and the 6th bit is the
// continuation bit. The continuation bit tells us whether there are more
// digits in this value following this digit.
//
//	Continuation
//	|    Sign
//	|    |
//	V    V
//	101011
const (
	vlqBaseShift = 5

	// binary: 100000
	vlqBase = 1 << vlqBaseShift

	// binary: 011111
	vlqBaseMask = vlqBase - 1

	// binary: 100000
	vlqContinuationBit = vlqBase
)

const base64Digits = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// toVLQSigned converts from a two-complement value to a value where the
// sign bit is placed in the least significant bit. For example, as
// decimals: 1 becomes 2 (10 binary), -1 becomes 3 (11 binary), 2 becomes
// 4 (100 binary), -2 becomes 5 (101 binary).
func toVLQSigned(value int) uint {
	if value < 0 {
		return uint(-value)<<1 + 1
	}
	return uint(value) << 1
}

// base64VLQEncode appends the base 64 VLQ encoded value.
func base64VLQEncode(b *strings.Builder, value int) {
	vlq := toVLQSigned(value)

	for {
		digit := vlq & vlqBaseMask
		vlq >>= vlqBaseShift
		if vlq > 0 {
			// There are still more digits in this value, so we must make sure the
			// continuation bit is marked.
			digit |= vlqContinuationBit
		}
		b.WriteByte(base64Digits[digit])
		if vlq == 0 {
			break
		}
	}
}

// writeJSONString writes s as JSON.stringify does: quotes, backslashes and
// control characters escaped, everything else as is.
func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(`\u00`)
				b.WriteString(strconv.FormatUint(uint64(c)>>4, 16))
				b.WriteString(strconv.FormatUint(uint64(c)&0xF, 16))
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
}

func writeNullableJSONString(b *strings.Builder, s string) {
	if s == "" {
		b.WriteString("null")
		return
	}
	writeJSONString(b, s)
}

func writeJSONStrings(b *strings.Builder, array []string) {
	b.WriteByte('[')
	for i, s := range array {
		if i > 0 {
			b.WriteByte(',')
		}
		writeJSONString(b, s)
	}
	b.WriteByte(']')
}
