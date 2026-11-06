package cli

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/docs"
)

func TestCommandHelpNamesReference(t *testing.T) {
	// These commands keep their own long description, so their help is not
	// the reference summary.
	handWritten := map[string]bool{
		fmtCommandName:                true,
		testTemplateMutationsName:     true,
		generateFakeServerCommandName: true,
	}
	help := func(t *testing.T, args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		err := commands(t.TempDir(), append(args, "--help"), func(string) string { return "" }, cliVersion, strings.NewReader(""), &stdout, io.Discard, runners{})
		require.NoError(t, err, "muxt %v --help", args)
		return stdout.String()
	}

	for _, name := range []string{
		generateCommandName, versionCommandName, checkCommandName,
		listTemplateCallersCommandName, listTemplateCallsCommandName,
		exploreModuleCommandName, generateFakeServerCommandName, testTemplateMutationsName,
		documentationCommandName, fmtCommandName,
	} {
		t.Run(name, func(t *testing.T) {
			out := help(t, name)
			if !handWritten[name] {
				summary, ok := docs.Summary(name)
				require.True(t, ok, "docs.Summary(%q)", name)
				assert.Contains(t, out, summary, "muxt %s --help", name)
			}
			line := "Full reference: muxt documentation docs/reference/commands/" + name + ".md"
			assert.True(t, slices.Contains(strings.Split(out, "\n"), line), "muxt %s --help has no line %q", name, line)
		})
	}

	for _, name := range []string{"completion", "help"} {
		t.Run(name+" has no reference page", func(t *testing.T) {
			assert.NotContains(t, help(t, name), "Full reference:", "muxt %s --help", name)
		})
	}
}
