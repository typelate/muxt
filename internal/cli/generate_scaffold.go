package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/muxtjson"
)

// generateConfigCommand is muxt generate config: it writes the empty muxt.json
// that generate package and a flagless generate then add entries to.
func generateConfigCommand(workingDirectory *string) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Write an empty muxt.json in the working directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			if err := muxtjson.Create(*workingDirectory); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "wrote "+muxtjson.FileName)
			return err
		},
	}
}
