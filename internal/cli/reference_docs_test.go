package cli

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/generate"
)

// referenceFlagCell matches each flag a table's first cell names, such as the
// two in "`-C, --change-directory`".
var referenceFlagCell = regexp.MustCompile("`--?([a-z][a-z-]*)`")

// referenceRows returns the table rows of the part of the page that starts at
// the line holding from and stops at the line holding to, as each row's
// backticked flag names in its first cell and the text of its second.
func referenceRows(t *testing.T, page, from, to string) [][2][]string {
	t.Helper()
	_, after, found := strings.Cut(page, from)
	require.True(t, found, "docs/reference/cli.md has no %q", from)
	section, _, _ := strings.Cut(after, to)
	var rows [][2][]string
	for line := range strings.SplitSeq(section, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(line, " | ")
		require.GreaterOrEqual(t, len(cells), 2, "row %q", line)
		var names, second []string
		for _, m := range referenceFlagCell.FindAllStringSubmatch(cells[0], -1) {
			names = append(names, m[1])
		}
		for _, m := range referenceFlagCell.FindAllStringSubmatch(cells[1], -1) {
			second = append(second, m[1])
		}
		rows = append(rows, [2][]string{names, second})
	}
	return rows
}

// TestCLIReferenceDocumentsEverySetting states that docs/reference/cli.md has
// a row for each setting generate takes, and that its deprecated table is the
// spellings the settings deprecate, each beside the setting it spells. A flag
// that only one command takes is on that command's page and is not checked.
func TestCLIReferenceDocumentsEverySetting(t *testing.T) {
	raw, err := os.ReadFile("../../docs/reference/cli.md")
	require.NoError(t, err, "reading docs/reference/cli.md")
	page := string(raw)

	documented := map[string]bool{}
	for _, row := range referenceRows(t, page, "## Flags", "### Deprecated flags") {
		for _, name := range row[0] {
			documented[name] = true
		}
	}
	wantDeprecated := map[string]string{deprecatedTemplatesVariable: useTemplatesVariable}
	var config generate.RoutesFileConfiguration
	for _, s := range generateSettings(&config) {
		assert.True(t, documented[s.name], "docs/reference/cli.md has no row for --%s", s.name)
		for _, spelling := range s.deprecated {
			wantDeprecated[spelling] = s.name
		}
	}

	gotDeprecated := map[string]string{}
	for _, row := range referenceRows(t, page, "### Deprecated flags", "## Troubleshooting") {
		require.Len(t, row[1], 1, "the deprecated table row for %v names one replacement", row[0])
		for _, old := range row[0] {
			gotDeprecated[old] = row[1][0]
		}
	}
	assert.Equal(t, wantDeprecated, gotDeprecated, "the deprecated table of docs/reference/cli.md, old spelling to setting")
}
