package docs

import (
	"path"
	"regexp"
	"strings"
)

// linkPattern matches the "](target)" part of an inline link or image. The
// target holds no whitespace or closing parenthesis.
var linkPattern = regexp.MustCompile(`\]\([^)\s]+\)`)

// RewriteLinks returns markdown with each inline link target that resolves to
// a path under docs rewritten as a repo-relative path. page is the repo-relative
// path of the file markdown came from, such as "docs/reference/cli.md". Targets
// are resolved against the directory of page, and a #fragment and a trailing
// slash are kept. Targets that leave docs, external URLs, and #fragment-only
// links are unchanged. Links inside fenced code blocks are unchanged.
//
// ref is not used yet; it will name the version in links that leave docs.
func RewriteLinks(page string, markdown []byte, _ string) []byte {
	var b strings.Builder
	inFence := false
	for line := range strings.SplitAfterSeq(string(markdown), "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		} else if !inFence {
			line = linkPattern.ReplaceAllStringFunc(line, func(link string) string {
				return "](" + rewriteTarget(page, link[2:len(link)-1]) + ")"
			})
		}
		b.WriteString(line)
	}
	return []byte(b.String())
}

// rewriteTarget returns target resolved against the directory of page when
// that resolves to a path under docs, and target unchanged otherwise.
func rewriteTarget(page, target string) string {
	p, fragment, hasFragment := strings.Cut(target, "#")
	if p == "" || strings.Contains(p, ":") {
		return target
	}
	resolved := path.Join(path.Dir(page), p)
	if resolved != "docs" && !strings.HasPrefix(resolved, "docs/") {
		return target
	}
	if strings.HasSuffix(p, "/") {
		resolved += "/"
	}
	if hasFragment {
		resolved += "#" + fragment
	}
	return resolved
}
