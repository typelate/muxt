package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// formatConfiguration is what muxt fmt is asked to do. As gofmt, it has no
// options: the flags say only where the result goes.
type formatConfiguration struct {
	Paths []string // as given; empty reads stdin
	Write bool     // -w, --write
	List  bool     // -l, --list
	Diff  bool     // -d, --diff
}

func formatCommand(workingDirectory *string, run func(*cobra.Command, string, formatConfiguration) error) *cobra.Command {
	var config formatConfiguration

	cmd := &cobra.Command{
		Use:   fmtCommandName + " [flags] [path ...]",
		Short: "Format Go HTML templates",
		Long: `Format Go HTML templates, as gofmt formats Go. With no paths, format
standard input; directories are walked for *.gohtml files.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if len(args) > 0 {
				config.Paths = args
			}
			if config.Write && len(config.Paths) == 0 {
				return errors.New("fmt: -w needs a file or directory; standard input has no file to write")
			}
			return run(cmd, *workingDirectory, config)
		},
	}

	cmd.Flags().BoolVarP(&config.Write, "write", "w", false, "write the result to the source file instead of standard output")
	cmd.Flags().BoolVarP(&config.List, "list", "l", false, "list files whose formatting differs")
	cmd.Flags().BoolVarP(&config.Diff, "diff", "d", false, "print a unified diff instead of the formatted source")

	return cmd
}
