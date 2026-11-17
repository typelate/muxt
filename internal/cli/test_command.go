package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/templatecover"
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
			// The package is the working directory or an entry of
			// muxt.json; what go test should run is said after --.
			before := args
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				before = args[:dash]
				config.GoTestArgs = args[dash:]
			}
			if len(before) > 0 {
				return fmt.Errorf("%s takes no arguments before --; pass go test flags after --: %s", testCommandName, strings.Join(before, " "))
			}
			if err := checkTestGoTestArgs(config.GoTestArgs); err != nil {
				return err
			}
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

// checkTestGoTestArgs refuses a go test flag that would undo the engine
// muxt test puts in place, before any package loads.
func checkTestGoTestArgs(args []string) error {
	purpose := templatecover.Purpose{
		Overlay:   "muxt test uses -overlay to put its template engine in place of html/template",
		ModSetter: "muxt test",
	}
	for f := range templatecover.Flags(args) {
		switch f.Name {
		case "overlay", "mod":
			return templatecover.Refuse(f, purpose)
		}
	}
	return nil
}
