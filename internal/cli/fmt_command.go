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
		Long: `Format Go HTML templates, as gofmt formats Go.

With no paths, fmt formats standard input to standard output. A path is a
file, formatted whatever its extension, or a directory, walked for
*.gohtml files (skipping testdata, vendor, and directories starting with
. or _). Without -w, -l, or -d, the formatted source is printed.

Whitespace a browser could render is kept as written. Inside a Go
package, fmt reads the stylesheets its templates link through the static
assets route (see muxt generate --input-static-assets-directory) and
their <style> elements; outside one, an element with a class or id is
left as written. A {{/* muxt:fmt off */}}, {{/* muxt:fmt on */}}, or
{{/* muxt:fmt preserve */}} comment opts a region out.

fmt exits 1 when a file cannot be read, parsed, or written, and with -l
or -d when any file's formatting differs.`,
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
