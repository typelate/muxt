package load_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/load"
)

func TestImportGraphOrder(t *testing.T) {
	for _, tt := range []struct {
		name  string
		graph load.ImportGraph
		want  []int
	}{
		{name: "empty", graph: load.ImportGraph{}, want: []int{}},
		{name: "no imports keeps order", graph: load.ImportGraph{nil, nil, nil}, want: []int{0, 1, 2}},
		{name: "importer follows imported", graph: load.ImportGraph{{1}, nil}, want: []int{1, 0}},
		{name: "chain", graph: load.ImportGraph{{1, 2}, {2}, nil}, want: []int{2, 1, 0}},
		{name: "independent ones keep their order", graph: load.ImportGraph{{2}, nil, nil}, want: []int{1, 2, 0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.graph.Order(), "ImportGraph.Order()")
		})
	}
}

func TestImportGraphImportsAny(t *testing.T) {
	graph := load.ImportGraph{{1}, nil, {0}}
	for _, tt := range []struct {
		name    string
		i       int
		written []int
		want    bool
	}{
		{name: "imports a written one", i: 0, written: []int{2, 1}, want: true},
		{name: "imports none written", i: 0, written: []int{2}, want: false},
		{name: "nothing written", i: 2, want: false},
		{name: "imports nothing", i: 1, written: []int{0, 2}, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, graph.ImportsAny(tt.i, tt.written), "ImportsAny(%d, %v)", tt.i, tt.written)
		})
	}
}
