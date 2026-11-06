package cli

import (
	"github.com/spf13/cobra"

	"github.com/typelate/muxt/docs"
)

// describe gives each direct subcommand of root that has a reference page a
// long description: the page's summary, unless the command already has a long
// description, followed by a line naming the page.
func describe(root *cobra.Command) {
	for _, cmd := range root.Commands() {
		summary, ok := docs.Summary(cmd.Name())
		if !ok {
			continue
		}
		if cmd.Long == "" {
			cmd.Long = summary
		}
		cmd.Long += "\n\nFull reference: muxt documentation docs/reference/commands/" + cmd.Name() + ".md"
	}
}
