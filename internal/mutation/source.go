package mutation

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"
)

// templateSource is a file whose bytes hold template text, together with
// what is needed to put mutated text back into that file.
//
// A template file holds its text directly. A template written as a Go
// string literal holds it encoded, so a mutation has to be spliced into
// the decoded text and the literal re-encoded around it, and a position
// in the text has to be mapped back through the encoding to address a
// real byte in the file.
type templateSource struct {
	// file is the absolute path of the file holding the text.
	file string

	// path is file relative to the directory the command ran in.
	path string

	// rootName is the name carried by the template the text itself
	// defines, as opposed to the ones its define clauses do.
	rootName string

	// text is the template text, decoded.
	text string

	// fileText is the whole file, as it is written.
	fileText string

	// litStart and litEnd bound text's encoding within fileText. For a
	// template file that is the whole file.
	litStart, litEnd int

	// offsets maps an offset in text to one in fileText. It is nil when
	// the two differ only by litStart, which is the case for a template
	// file and for a raw literal holding no carriage returns.
	offsets []int

	// encode turns mutated text back into the bytes that go between
	// litStart and litEnd.
	encode func(string) string

	// lines indexes fileText, so positions are reported in the file a
	// reader would open.
	lines lineIndex

	// leftDelim and rightDelim are the delimiters this text was written
	// with. A template set built with Delims keeps them to itself, so
	// they are recovered from the clauses check located: a definition's
	// End span runs from the left delimiter through the right one, which
	// is enough to read both off.
	//
	// Empty means the text/template defaults.
	leftDelim  string
	rightDelim string

	// regions are the actions written in text, scanned once because
	// every template defined here shares them.
	regions []region
}

// spaceChars are the bytes text/template treats as whitespace beside a
// trim marker.
const spaceChars = " \t\r\n"

// delimiters reads the delimiters a definition was written with off the
// clause that closes it.
//
// An end clause is the one place the shape is fixed: the left delimiter,
// an optional trim marker, the word end, another optional marker, and
// the right delimiter. Nothing else in it varies, so whatever surrounds
// the word is the pair.
func delimiters(text string, definition check.Definition) (left, right string, ok bool) {
	if !definition.TemplateName.IsValid() {
		// A template with no define clause has no end clause either.
		return "", "", false
	}
	start, end := definition.End.Offset, definition.End.Offset+definition.End.Length
	if start < 0 || end > len(text) {
		return "", "", false
	}
	before, after, found := strings.Cut(text[start:end], "end")
	if !found {
		return "", "", false
	}
	left = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(before, spaceChars), "-"), spaceChars)
	right = strings.TrimLeft(strings.TrimPrefix(strings.TrimLeft(after, spaceChars), "-"), spaceChars)
	if left == "" || right == "" {
		return "", "", false
	}
	return left, right, true
}

// mutatedText returns the template text with the edits in place, which
// is what has to parse and type check for the mutant to be worth running.
//
// The edits must be sorted by start and must not overlap, which is what
// building them from distinct operands of one action guarantees.
func (s *templateSource) mutatedText(edits []edit) string {
	var b strings.Builder
	last := 0
	for _, e := range edits {
		if e.start < last || e.end > len(s.text) || e.start > e.end {
			return s.text
		}
		b.WriteString(s.text[last:e.start])
		b.WriteString(e.text)
		last = e.end
	}
	b.WriteString(s.text[last:])
	return b.String()
}

// newFileSource builds a source for a template file, whose text is its
// bytes.
func newFileSource(file, path, fileText, leftDelim, rightDelim string) *templateSource {
	return &templateSource{
		file:       file,
		path:       path,
		rootName:   filepath.Base(file),
		text:       fileText,
		fileText:   fileText,
		litEnd:     len(fileText),
		encode:     func(mutated string) string { return mutated },
		lines:      newLineIndex(fileText),
		leftDelim:  leftDelim,
		rightDelim: rightDelim,
		regions:    regions(fileText, leftDelim, rightDelim),
	}
}

// newLiteralSource builds a source for a template written as a Go string
// literal spanning litStart to litEnd in fileText.
func newLiteralSource(file, path, rootName, fileText, leftDelim, rightDelim string, litStart, litEnd int) (*templateSource, error) {
	if litStart < 0 || litEnd > len(fileText) || litStart >= litEnd {
		return nil, fmt.Errorf("%s: string literal is not within the file", path)
	}
	literal := fileText[litStart:litEnd]
	text, err := strconv.Unquote(literal)
	if err != nil {
		return nil, fmt.Errorf("%s: reading string literal: %w", path, err)
	}
	offsets, err := literalOffsets(literal, text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i := range offsets {
		offsets[i] += litStart
	}
	return &templateSource{
		file:       file,
		path:       path,
		rootName:   rootName,
		text:       text,
		fileText:   fileText,
		litStart:   litStart,
		litEnd:     litEnd,
		offsets:    offsets,
		encode:     literalEncoder(literal),
		lines:      newLineIndex(fileText),
		leftDelim:  leftDelim,
		rightDelim: rightDelim,
		regions:    regions(text, leftDelim, rightDelim),
	}, nil
}

// fileOffset maps an offset in the template text to one in the file.
func (s *templateSource) fileOffset(offset int) int {
	if s.offsets == nil {
		return s.litStart + offset
	}
	if offset < 0 || offset >= len(s.offsets) {
		return s.litStart
	}
	return s.offsets[offset]
}

// apply returns the whole file with the edits in place.
func (s *templateSource) apply(edits []edit) string {
	return s.fileText[:s.litStart] + s.encode(s.mutatedText(edits)) + s.fileText[s.litEnd:]
}

// literalEncoder returns a function writing text as a Go string literal,
// keeping the quoting the original used where it still works.
//
// A raw literal stays raw, which keeps a mutant's diff readable, unless
// the mutated text holds a back quote or a carriage return, neither of
// which a raw literal can carry.
func literalEncoder(literal string) func(string) string {
	if strings.HasPrefix(literal, "`") {
		return func(mutated string) string {
			if !strings.ContainsAny(mutated, "`\r") {
				return "`" + mutated + "`"
			}
			return strconv.Quote(mutated)
		}
	}
	return strconv.Quote
}

// literalOffsets maps each byte of a string literal's value to the offset
// of the source byte it was decoded from, with one final entry for the
// position just past the value.
//
// The mapping is not the identity even for a raw literal, because
// go/scanner drops carriage returns from a raw literal's value while
// positions keep addressing the file.
func literalOffsets(literal, value string) ([]int, error) {
	offsets := make([]int, 0, len(value)+1)

	if strings.HasPrefix(literal, "`") {
		for i := 1; i < len(literal)-1; i++ {
			if literal[i] == '\r' {
				continue
			}
			offsets = append(offsets, i)
		}
		offsets = append(offsets, len(literal)-1)
		if len(offsets) != len(value)+1 {
			return nil, fmt.Errorf("raw string literal does not decode to its value")
		}
		return offsets, nil
	}

	for i := 1; i < len(literal)-1; {
		r, multibyte, tail, err := strconv.UnquoteChar(literal[i:], '"')
		if err != nil {
			return nil, fmt.Errorf("reading string literal: %w", err)
		}
		width := 1
		if multibyte {
			width = utf8.RuneLen(r)
		}
		for range width {
			offsets = append(offsets, i)
		}
		i = len(literal) - len(tail)
	}
	offsets = append(offsets, len(literal)-1)
	if len(offsets) != len(value)+1 {
		return nil, fmt.Errorf("string literal does not decode to its value")
	}
	return offsets, nil
}

// findStringLiteral returns the Go string literal covering offset in the
// named file, which is the literal a template written in Go source was
// written as.
func findStringLiteral(pl []*packages.Package, filename string, offset int) (start, end int, ok bool) {
	seen := make(map[*ast.File]struct{})
	for _, pkg := range pl {
		for _, file := range pkg.Syntax {
			if _, done := seen[file]; done {
				continue
			}
			seen[file] = struct{}{}
			tokenFile := pkg.Fset.File(file.Pos())
			if tokenFile == nil || tokenFile.Name() != filename {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				lit, isLit := node.(*ast.BasicLit)
				if !isLit || lit.Kind != token.STRING {
					return true
				}
				litStart, litEnd := tokenFile.Offset(lit.Pos()), tokenFile.Offset(lit.End())
				if offset < litStart || offset >= litEnd {
					return true
				}
				// Nested literals do not occur, so the first match is
				// the one wanted.
				start, end, ok = litStart, litEnd, true
				return false
			})
			if ok {
				return start, end, true
			}
		}
	}
	return 0, 0, false
}
