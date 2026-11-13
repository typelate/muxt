package load

import "slices"

// ImportGraph is plain data: ImportGraph[i] holds the indexes i imports. It
// is what a run reads to order its packages without holding go/packages.
type ImportGraph [][]int

// Order returns every index after the indexes it imports; indexes with no
// import between them keep their order. The go command rejects import cycles,
// so there is always an index to take next.
func (g ImportGraph) Order() []int {
	remaining := make([]int, len(g))
	for i := range remaining {
		remaining[i] = i
	}
	ordered := make([]int, 0, len(g))
	for len(remaining) > 0 {
		next := slices.IndexFunc(remaining, func(i int) bool {
			return !slices.ContainsFunc(remaining, func(dependency int) bool {
				return slices.Contains(g[i], dependency)
			})
		})
		next = max(next, 0)
		ordered = append(ordered, remaining[next])
		remaining = slices.Delete(remaining, next, next+1)
	}
	return ordered
}

// ImportsAny reports whether i imports any of written, so a load made before
// they were written is stale for i.
func (g ImportGraph) ImportsAny(i int, written []int) bool {
	return slices.ContainsFunc(written, func(w int) bool { return slices.Contains(g[i], w) })
}
