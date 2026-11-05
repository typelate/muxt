package analysis

import "strings"

type textEditKind int

const (
	// escapedLT is a < html/template wrote as &lt;.
	escapedLT textEditKind = iota + 1
	// stripped is a comment html/template left out.
	stripped
)

// textEdit is a change html/template's escaper made to the text of a text
// node, at the byte offsets [start, end) of the source text.
type textEdit struct {
	kind       textEditKind
	start, end int
	// closed reports that a stripped comment ends in the text, at its
	// closing delimiter; one that is not runs to the end of the text.
	closed bool
}

// textEdits returns the changes that make what html/template's escaper
// wrote for a text node, from its source text. They are the escaper's
// only rewrites of text: a < written as &lt; (or as \x3C in a script
// string), and a comment stripped, an HTML, CSS or JavaScript one, which
// may have started before the text or end after it, and a CSS or
// JavaScript one written as a space or newline. It reports false when no
// such changes make written from source.
func textEdits(source, written string) ([]textEdit, bool) {
	a := aligner{source: source, written: written, failed: make(map[[2]int]bool)}
	if !a.align(0, 0) {
		return nil, false
	}
	return a.edits, true
}

// aligner searches, left to right, for the edits that make written from
// source. A match is tried before an edit, except at the start of the
// text, where a comment open before the text is tried first: "y-->y"
// written as "y" is the end of a comment, not a JavaScript --> comment.
type aligner struct {
	source, written string
	edits           []textEdit
	// failed are the offsets, into source and written, from which no
	// edits align the rest.
	failed map[[2]int]bool
}

func (a *aligner) align(i, j int) bool {
	if i == len(a.source) {
		return j == len(a.written)
	}
	if a.failed[[2]int{i, j}] {
		return false
	}
	n := len(a.edits)
	if i == 0 && a.strip(i, j) {
		return true
	}
	if j < len(a.written) && a.source[i] == a.written[j] && a.align(i+1, j+1) {
		return true
	}
	if a.source[i] == '<' {
		rest := a.written[j:]
		if strings.HasPrefix(rest, "&lt;") {
			a.edits = append(a.edits, textEdit{kind: escapedLT, start: i, end: i + 1})
			if a.align(i+1, j+len("&lt;")) {
				return true
			}
			a.edits = a.edits[:n]
		}
		if strings.HasPrefix(rest, `\x3C`) && a.align(i+1, j+len(`\x3C`)) {
			return true
		}
	}
	if i != 0 && a.strip(i, j) {
		return true
	}
	a.failed[[2]int{i, j}] = true
	return false
}

// strip tries the comments that may be stripped from offset i.
func (a *aligner) strip(i, j int) bool {
	n := len(a.edits)
	for _, end := range a.commentEnds(i) {
		if end.offset == i {
			continue // a line comment that is empty
		}
		a.edits = append(a.edits, textEdit{kind: stripped, start: i, end: end.offset, closed: end.closed})
		if a.align(end.offset, j) {
			return true
		}
		// A CSS or JavaScript comment is written as a space or newline.
		if j < len(a.written) && (a.written[j] == ' ' || a.written[j] == '\n') && a.align(end.offset, j+1) {
			return true
		}
		a.edits = a.edits[:n]
	}
	return false
}

type commentEnd struct {
	offset int
	closed bool
}

// commentEnds returns where a comment stripped from offset i of the
// source may end: after its closing delimiter, or at the end of the text.
// One starts at a comment's opening delimiter, or at the start of the
// text when a comment is open there.
func (a *aligner) commentEnds(i int) []commentEnd {
	rest := a.source[i:]
	var ends []commentEnd
	closeAt := func(from int, delim string) {
		if k := strings.Index(rest[from:], delim); k >= 0 {
			ends = append(ends, commentEnd{offset: i + from + k + len(delim), closed: true})
		}
	}
	lineEnd := func() {
		if k := strings.IndexAny(rest, "\n\r\u2028\u2029"); k >= 0 {
			ends = append(ends, commentEnd{offset: i + k, closed: true})
		}
	}
	opens := false
	switch {
	case strings.HasPrefix(rest, commentOpen):
		// An HTML comment, or a JavaScript line comment.
		closeAt(len(commentOpen), commentClose)
		lineEnd()
		opens = true
	case strings.HasPrefix(rest, "/*"):
		closeAt(len("/*"), "*/")
		opens = true
	case strings.HasPrefix(rest, "//"), strings.HasPrefix(rest, commentClose):
		lineEnd()
		opens = true
	}
	if i == 0 {
		closeAt(0, commentClose)
		closeAt(0, "*/")
		lineEnd()
		opens = true
	}
	if !opens {
		return nil
	}
	return append(ends, commentEnd{offset: len(a.source)})
}
