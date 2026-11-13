package cli

import (
	"github.com/spf13/cobra"

	"github.com/typelate/muxt/docs"
)

// describe gives each direct subcommand of root that has a reference page a
// long description: the page's summary, unless the command already has a long
// description, followed by a line naming the page. Its own sub-commands get
// the same line after their short description.
func describe(root *cobra.Command) {
	for _, cmd := range root.Commands() {
		summary, ok := docs.Summary(cmd.Name())
		if !ok {
			continue
		}
		if cmd.Long == "" {
			cmd.Long = summary
		}
		reference := "\n\nFull reference: muxt documentation docs/reference/commands/" + cmd.Name() + ".md"
		cmd.Long += reference
		// A sub-command is documented on its parent's page.
		for _, sub := range cmd.Commands() {
			if sub.Long == "" {
				sub.Long = sub.Short
			}
			sub.Long += reference
		}
	}
}

// generateLong is the reference summary of generate followed by the one fact a
// reader of --help needs before the flags: with none, it reads muxt.json.
func generateLong() string {
	summary, ok := docs.Summary(generateCommandName)
	if !ok {
		return ""
	}
	return summary + "\n\nWith no flags it reads muxt.json: muxt documentation docs/reference/muxt-json.md"
}
