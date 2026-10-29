// Ported from lib/tokenizer/OffsetToLocation.js.

package tokenizer

import "unicode/utf8"

// Position is a point in a source: upstream's {offset, line, column}.
// Offsets and columns count bytes; lines and columns start at 1 unless
// SetSource says otherwise.
type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

// SourcePosition is what GetLocation returns: upstream's
// {source, offset, line, column}.
type SourcePosition struct {
	Source string `json:"source"`
	Position
}

// Location is a range in a source, upstream's node.loc:
// {source, start: {offset, line, column}, end: {offset, line, column}}.
type Location struct {
	Source string   `json:"source"`
	Start  Position `json:"start"`
	End    Position `json:"end"`
}

const (
	lf = 10 // upstream N
	ff = 12 // upstream F
	cr = 13 // upstream R
)

func computeLinesAndColumns(host *OffsetToLocation) {
	source := host.Source
	sourceLength := len(source)
	startOffset := 0
	if len(source) > 0 {
		first, _ := utf8.DecodeRuneInString(source)
		startOffset = IsBOM(int(first))
	}
	// Upstream adopts sourceLength cells and then writes cell sourceLength,
	// which a typed array silently drops; ask for one more.
	lines := adoptBuffer(host.lines, sourceLength+1)
	columns := adoptBuffer(host.columns, sourceLength+1)
	line := host.StartLine
	column := host.StartColumn

	for i := startOffset; i < sourceLength; i++ {
		code := source[i]

		lines[i] = line
		columns[i] = column
		column++

		if code == lf || code == cr || code == ff {
			if code == cr && i+1 < sourceLength && source[i+1] == lf {
				i++
				lines[i] = line
				columns[i] = column
			}

			line++
			column = 1
		}
	}

	lines[sourceLength] = line
	columns[sourceLength] = column

	host.lines = lines
	host.columns = columns
	host.computed = true
}

// OffsetToLocation converts byte offsets in a source to lines and columns.
// The tables are computed lazily on the first lookup after SetSource.
type OffsetToLocation struct {
	Source      string
	StartOffset int
	StartLine   int
	StartColumn int

	computed bool
	lines    []int
	columns  []int
}

// NewOffsetToLocation returns an OffsetToLocation for source. Upstream's
// defaults for omitted arguments are ("", 0, 1, 1).
func NewOffsetToLocation(source string, startOffset, startLine, startColumn int) *OffsetToLocation {
	l := new(OffsetToLocation)
	l.SetSource(source, startOffset, startLine, startColumn)
	l.lines = nil
	l.columns = nil
	return l
}

// SetSource resets the converter for a new source. startOffset is added to
// every reported offset; startLine and startColumn are the line and column
// of the first char (upstream defaults them to 1).
func (l *OffsetToLocation) SetSource(source string, startOffset, startLine, startColumn int) {
	l.Source = source
	l.StartOffset = startOffset
	l.StartLine = startLine
	l.StartColumn = startColumn
	l.computed = false
}

// GetLocation returns the position of offset, which must be in
// [0, len(l.Source)].
func (l *OffsetToLocation) GetLocation(offset int, filename string) SourcePosition {
	if !l.computed {
		computeLinesAndColumns(l)
	}

	return SourcePosition{
		Source: filename,
		Position: Position{
			Offset: l.StartOffset + offset,
			Line:   l.lines[offset],
			Column: l.columns[offset],
		},
	}
}

// GetLocationRange returns the location of [start, end), both of which must
// be in [0, len(l.Source)].
func (l *OffsetToLocation) GetLocationRange(start, end int, filename string) *Location {
	if !l.computed {
		computeLinesAndColumns(l)
	}

	return &Location{
		Source: filename,
		Start: Position{
			Offset: l.StartOffset + start,
			Line:   l.lines[start],
			Column: l.columns[start],
		},
		End: Position{
			Offset: l.StartOffset + end,
			Line:   l.lines[end],
			Column: l.columns[end],
		},
	}
}
