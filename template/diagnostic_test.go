package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiagnosticString(t *testing.T) {
	d := Diagnostic{Pos: 3, Severity: SeverityWarning, Code: DiagOrphanEndTag, Message: `unmatched </b> is ignored by browsers`}
	assert.Equal(t, `warning at pos 3: unmatched </b> is ignored by browsers`, d.String())
}

func TestDiagnosticsSortByPos(t *testing.T) {
	ds := Diagnostics{{Pos: 9}, {Pos: 2}, {Pos: 5}}
	ds.SortByPos()
	assert.Equal(t, Diagnostics{{Pos: 2}, {Pos: 5}, {Pos: 9}}, ds)
}
