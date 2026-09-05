package astgen

import "strings"

// NearestString returns the candidate closest to target by Levenshtein
// distance when that distance is small enough to look like a typo: at
// most one edit for short names, up to a third of the target's length
// for longer ones. The comparison is case-insensitive so a wrong-case
// spelling still finds its match.
func NearestString(target string, candidates []string) (string, bool) {
	best, bestDistance := "", -1
	lowerTarget := strings.ToLower(target)
	for _, candidate := range candidates {
		d := levenshtein(lowerTarget, strings.ToLower(candidate))
		if bestDistance < 0 || d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	if bestDistance <= 0 && best == target {
		// An exact match is not a suggestion.
		return "", false
	}
	if bestDistance < 0 || bestDistance > max(1, len(target)/3) {
		return "", false
	}
	return best, true
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	previous := make([]int, len(rb)+1)
	current := make([]int, len(rb)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		current[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(rb)]
}
