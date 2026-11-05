package analysis

import (
	"slices"
	"testing"
)

func TestTextEdits(t *testing.T) {
	for _, test := range []struct {
		source, written string
		want            []textEdit
		ok              bool
	}{
		{source: "a", written: "a", ok: true},
		{source: "a<b", written: "a&lt;b", want: []textEdit{{kind: escapedLT, start: 1, end: 2}}, ok: true},
		{source: "a<!-- c -->b", written: "ab", want: []textEdit{{kind: stripped, start: 1, end: 11, closed: true}}, ok: true},
		{source: "a<!-- c", written: "a", want: []textEdit{{kind: stripped, start: 1, end: 7}}, ok: true},
		{source: " c -->b", written: "b", want: []textEdit{{kind: stripped, start: 0, end: 6, closed: true}}, ok: true},
		{source: "y-->y", written: "y", want: []textEdit{{kind: stripped, start: 0, end: 4, closed: true}}, ok: true},
		{source: "a /* c */ b", written: "a   b", want: []textEdit{{kind: stripped, start: 2, end: 9, closed: true}}, ok: true},
		{source: "a", written: "b"},
	} {
		got, ok := textEdits(test.source, test.written)
		if ok != test.ok || !slices.Equal(got, test.want) {
			t.Errorf("textEdits(%q, %q) = %v, %t; want %v, %t", test.source, test.written, got, ok, test.want, test.ok)
		}
	}
}
