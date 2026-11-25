package analysis

import (
	"cmp"

	"github.com/typelate/htt"

	"github.com/typelate/muxt/internal/diagnostic"
)

// diagnosticAt converts d, a finding at byte offsets of tree, to the
// diagnostic muxt reports: htt knows no file and no line, so the place is
// built here, where the tree is at hand. The file is the path the tree was
// read from, or the name it was parsed under when it has none.
func diagnosticAt(tree *htt.Tree, d htt.Diagnostic) diagnostic.Diagnostic {
	start := tree.PositionAt(d.Pos)
	place := diagnostic.Place{
		File:   cmp.Or(tree.Filepath, start.ParseName),
		Line:   start.Line,
		Column: start.Column,
	}
	if d.End > d.Pos {
		end := tree.PositionAt(d.End)
		place.EndLine, place.EndColumn = end.Line, end.Column
	}
	severity := diagnostic.Warning
	if d.Severity == htt.SeverityError {
		severity = diagnostic.Error
	}
	return diagnostic.Diagnostic{
		Severity: severity,
		Code:     string(d.Code),
		Place:    place,
		Message:  d.Message,
	}
}
