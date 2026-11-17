package cli

import (
	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/generate"
)

// testConfiguration is what the command line gives muxt test.
type testConfiguration struct {
	// GoTestArgs is everything after --, as written.
	GoTestArgs []string
}

// testCommand runs the tests of the packages it selects on muxt's template
// engine. It takes no setting flags: the templates variable of a package is
// only read to attribute a profile, and the default is what a package that
// muxt.json does not configure has.
func testCommand(workingDirectory *string, getEnv func(string) string, run func(*cobra.Command, []packageJob, testConfiguration) error) *cobra.Command {
	var config testConfiguration

	cmd := &cobra.Command{
		Use:   testCommandName + " [-- go test flags]",
		Short: "Run go test on muxt's template engine",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			spec := jobSpec{
				Name:  testCommandName,
				Scope: eachEntry,
				Fallback: func(dir string) (generate.RoutesFileConfiguration, error) {
					return defaultConfiguration(dir, getEnv)
				},
			}
			sel, err := selectJobs(spec, *workingDirectory, cmd.Flags(), getEnv)
			if err != nil {
				return err
			}
			return run(cmd, sel.Jobs, config)
		},
	}

	return cmd
}
