package docs

import (
	"path"
	"regexp"
	"strings"
)

// linkPattern matches the "](target)" part of an inline link or image. The
// target holds no whitespace or closing parenthesis.
var linkPattern = regexp.MustCompile(`\]\([^)\s]+\)`)

// blobURL is the GitHub URL that a repo-relative path is appended to, after a
// ref, to reach the file on GitHub.
const blobURL = "https://github.com/typelate/muxt/blob/"

// RewriteLinks returns markdown with each inline link target that resolves to
// a path under docs rewritten as a repo-relative path. page is the repo-relative
// path of the file markdown came from, such as "docs/reference/cli.md". Targets
// are resolved against the directory of page, and a #fragment and a trailing
// slash are kept. A target that leaves docs becomes a GitHub URL for that path
// at ref, such as "main" or "v1.2.3". External URLs and #fragment-only links
// are unchanged. Links inside fenced code blocks are unchanged.
func RewriteLinks(page string, markdown []byte, ref string) []byte {
	var b strings.Builder
	inFence := false
	for line := range strings.SplitAfterSeq(string(markdown), "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		} else if !inFence {
			line = linkPattern.ReplaceAllStringFunc(line, func(link string) string {
				return "](" + rewriteTarget(page, link[2:len(link)-1], ref) + ")"
			})
		}
		b.WriteString(line)
	}
	return []byte(b.String())
}

// rewriteTarget returns target resolved against the directory of page. The
// result is a path under docs, or a GitHub URL at ref when the path leaves
// docs. External targets are returned unchanged.
func rewriteTarget(page, target, ref string) string {
	p, fragment, hasFragment := strings.Cut(target, "#")
	if p == "" || strings.Contains(p, ":") {
		return target
	}
	resolved := path.Join(path.Dir(page), p)
	if strings.HasSuffix(p, "/") {
		resolved += "/"
	}
	if resolved != "docs" && !strings.HasPrefix(resolved, "docs/") {
		resolved = blobURL + ref + "/" + resolved
	}
	if hasFragment {
		resolved += "#" + fragment
	}
	return resolved
}
