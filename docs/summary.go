package docs

import (
	"regexp"
	"strings"
)

// summaryLink matches an inline link, capturing its text.
var summaryLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)

// Summary returns the first paragraph of the reference page for command, as
// reference/commands/<command>.md. The paragraph is one line, with its links
// replaced by their text. ok is false when no such page is embedded or the page
// has no paragraph after its heading.
func Summary(command string) (string, bool) {
	content, err := files.ReadFile("reference/commands/" + command + ".md")
	if err != nil {
		return "", false
	}
	var lines []string
	afterHeading := false
	for line := range strings.SplitSeq(string(content), "\n") {
		if !afterHeading {
			afterHeading = strings.HasPrefix(line, "# ")
			continue
		}
		if strings.TrimSpace(line) == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	if len(lines) == 0 {
		return "", false
	}
	return summaryLink.ReplaceAllString(strings.Join(lines, " "), "$1"), true
}
