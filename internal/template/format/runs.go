package format

// runClass is what the browser does with a whitespace run, and so what
// the formatter may rewrite it to (v2 spec §2).
type runClass uint8

const (
	runPreserved      runClass = iota // emitted byte for byte
	runTrimmed                        // removed at execution by a trim marker
	runCollapse                       // its owner collapses white space: amount free, kind fixed
	runPreserveBreaks                 // pre-line: spaces or the same number of breaks
)

// segment is a maximal span of text that is all whitespace run bytes
// (U+0020, U+0009, U+000A) or all other bytes. Source has written
// every CR as LF before parsing.
type segment struct {
	start, end int
	space      bool
}

func splitRuns(text []byte) []segment {
	var segments []segment
	for i := 0; i < len(text); {
		space := isRunByte(text[i])
		j := i + 1
		for j < len(text) && isRunByte(text[j]) == space {
			j++
		}
		segments = append(segments, segment{start: i, end: j, space: space})
		i = j
	}
	return segments
}

func isRunByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n'
}

// isRunOnly reports whether text is non-empty and all whitespace run bytes.
func isRunOnly(text []byte) bool {
	segments := splitRuns(text)
	return len(segments) == 1 && segments[0].space
}
