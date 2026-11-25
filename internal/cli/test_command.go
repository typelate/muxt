package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/templatecover"
)

// testConfiguration is what the command line gives muxt test.
type testConfiguration struct {
	// CoverProfile is the file to write the template blocks the tests
	// reached to, as a Go cover profile; empty for none.
	CoverProfile string
	// GoTestArgs is everything after --, as written.
	GoTestArgs []string
}

// testCommand runs the tests of the packages it selects on muxt's template
// engine. It takes no setting flags: the templates variable of a package is
// only read to attribute a profile, and the default is what a package that
// muxt.json does not configure has.
func testCommand(workingDirectory *string, getEnv func(string) string, run func(*cobra.Command, []packageRun, testConfiguration) error) *cobra.Command {
	var config testConfiguration

	cmd := &cobra.Command{
		Use:   testCommandName + " [-- go test flags]",
		Short: "Run go test on muxt's template engine; write template coverage with --output-cover-profile",
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
			if config.CoverProfile != "" {
				if err := checkProfilePackages(config.GoTestArgs); err != nil {
					return err
				}
				// A relative path means relative to -C, not to the
				// directory the process started in, which is where the
				// runner would create it.
				config.CoverProfile = absoluteDir(*workingDirectory, config.CoverProfile)
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
			return run(cmd, sel.Packages, config)
		},
	}

	cmd.Flags().StringVar(&config.CoverProfile, outputCoverProfile, "", "write the template blocks the tests reached to this file, as a Go cover profile for go tool cover -html")

	return cmd
}

// checkProfilePackages refuses a package pattern after --. A profile
// attributes a block to the template files of the package whose go test
// reached it, and one go test over several packages gives one cover
// directory for all of them, where two packages' index.gohtml would meet.
func checkProfilePackages(goTestArgs []string) error {
	// go test takes its own flags spelled -test.NAME too, and the shared
	// walk knows which take a value only by their short names. Read under
	// the short name here, so a value such as the pattern of -test.run is
	// not taken for a package; the walk itself is test-template-mutations'
	// as well and stays as it is.
	short := make([]string, len(goTestArgs))
	for i, arg := range goTestArgs {
		short[i] = arg
		for _, prefix := range []string{"--test.", "-test."} {
			if rest, ok := strings.CutPrefix(arg, prefix); ok {
				short[i] = "-" + rest
				break
			}
		}
	}
	var packages []string
	for f := range templatecover.Flags(short) {
		if f.Name == "" {
			packages = append(packages, f.Args...)
		}
	}
	if len(packages) > 0 {
		return fmt.Errorf("--%s needs one package per go test; pass no packages after --, and run in the package directory or list the packages in muxt.json: %s", outputCoverProfile, strings.Join(packages, " "))
	}
	return nil
}

// checkTestGoTestArgs refuses a go test flag that would undo the engine
// muxt test puts in place, before any package loads.
func checkTestGoTestArgs(args []string) error {
	purpose := templatecover.Purpose{
		Overlay:   "muxt test uses -overlay to put its template engine in place of html/template",
		ModSetter: "muxt test",
	}
	walk := slices.Collect(templatecover.Flags(args))
	for i, f := range walk {
		if f.Name == "C" {
			// go test reads -C only as its first flag, and muxt puts its
			// own flags first, so muxt's -C is the one that can work. The
			// shared walk does not read -C's next argument as its value,
			// because test-template-mutations passes -C on for go test to
			// refuse, so the directory is read here.
			dir := f.Value
			if !f.HasValue && i+1 < len(walk) && walk[i+1].Name == "" {
				dir = walk[i+1].Args[0]
			}
			if dir == "" {
				return errors.New("-C after -- cannot reach go test, which takes it only as its first flag; use muxt's own -C before test")
			}
			return fmt.Errorf("-C after -- cannot reach go test, which takes it only as its first flag; use muxt -C %s test", dir)
		}
		if err := templatecover.Refuse(f, purpose); err != nil {
			return err
		}
	}
	return nil
}
