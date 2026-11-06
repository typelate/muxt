package cli

import (
	"github.com/spf13/cobra"

	"github.com/typelate/muxt/docs"
)

const documentationCommandName = "documentation"

// documentationCommand prints the docs embedded in the binary, so a project
// that has only the installed muxt can read the rules without its source.
func documentationCommand() *cobra.Command {
	return &cobra.Command{
		Use:     documentationCommandName + " [path]",
		Aliases: []string{"docs", "d"},
		Short:   "Print muxt's documentation by path (no path lists every file)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			var arg string
			if len(args) == 1 {
				arg = args[0]
			}
			entry, err := docs.Resolve(arg)
			if err != nil {
				return err
			}
			content, err := entry.Render("")
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(content)
			return err
		},
	}
}
