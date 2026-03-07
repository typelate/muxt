package cli

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/charmbracelet/glamour"

	"github.com/typelate/muxt/docs"
)

var markdownLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)

// commandHelp reads and renders markdown files from the embedded docs FS.
// It concatenates all specified files and renders them with glamour for terminal output.
func commandHelp(docFiles ...string) string {
	docFS := docs.Markdown()
	baseURL := githubBlobURL()

	var buf strings.Builder
	for _, name := range docFiles {
		data, err := fs.ReadFile(docFS, name)
		if err != nil {
			continue
		}
		content := rewriteDocLinks(string(data), name, baseURL)
		buf.WriteString(content)
		buf.WriteString("\n\n")
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
	)
	if err != nil {
		return buf.String()
	}

	out, err := r.Render(buf.String())
	if err != nil {
		return buf.String()
	}

	return strings.TrimSpace(out)
}

func rewriteDocLinks(markdown string, docPath string, baseURL string) string {
	docDir := path.Dir("docs/" + docPath)

	return markdownLinkRe.ReplaceAllStringFunc(markdown, func(match string) string {
		submatches := markdownLinkRe.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}
		text := submatches[1]
		href := submatches[2]

		if strings.HasPrefix(href, "http") || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") {
			return match
		}

		resolved := path.Clean(path.Join(docDir, href))
		return fmt.Sprintf("[%s](%s/%s)", text, baseURL, resolved)
	})
}

func githubBlobURL() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok || strings.Contains(bi.Main.Version, "+dirty") {
		return "https://github.com/typelate/muxt/blob/main"
	}

	version := bi.Main.Version
	if version == "" || version == "(devel)" {
		version = "main"
	}

	return fmt.Sprintf("https://%s/blob/%s", bi.Main.Path, version)
}
