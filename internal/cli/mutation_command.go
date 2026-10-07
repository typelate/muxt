package cli

import (
	"fmt"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/mutation"
)

// testTemplateMutationsCommand varies each dynamic and control flow
// action in the project's templates and reports the variations the tests
// let through.
func testTemplateMutationsCommand(workingDirectory *string, run func(*cobra.Command, string, mutation.Configuration) error) *cobra.Command {
	var (
		config                 mutation.Configuration
		templatePattern        string
		runPattern             string
		deprecatedTemplatesVar string
	)

	cmd := &cobra.Command{
		Use:   testTemplateMutationsName + " [packages] [-- go test flags]",
		Short: "Vary template actions and report the ones no test catches",
		Long: `Vary each dynamic and control flow action in the project's templates, one
at a time, and re-run the tests against each variation.

A variation the tests still pass through is a miss: nothing the suite
asserts on depends on what that action does. A variation that makes a
test fail is caught, which is the outcome to want.

The tests run once unmutated first. If that baseline fails, nothing is
mutated, because every later failure would be indistinguishable from the
one already there.

Variations are delivered through the go command's -overlay flag, so the
working tree is never written to.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar); err != nil {
				return err
			}
			if err := checkTemplatesVariables(config.TemplatesVariables); err != nil {
				return err
			}

			if templatePattern != "" {
				pattern, err := regexp.Compile(templatePattern)
				if err != nil {
					return fmt.Errorf("--template-pattern: %w", err)
				}
				config.TemplatePattern = pattern
			}
			if runPattern != "" {
				pattern, err := regexp.Compile(runPattern)
				if err != nil {
					return fmt.Errorf("--run: %w", err)
				}
				config.Run = pattern
			}
			// Everything before a -- names packages; everything after it
			// is handed to go test as written.
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				config.Packages = args[:dash]
				config.GoTestArgs = args[dash:]
			} else {
				config.Packages = args
			}
			if err := mutation.CheckGoTestArgs(config.GoTestArgs); err != nil {
				return err
			}
			config.SeedSet = cmd.Flags().Changed("seed")
			return run(cmd, *workingDirectory, config)
		},
	}

	addUseTemplatesVarToFlagSet(cmd.Flags(), &config.TemplatesVariables, &deprecatedTemplatesVar)
	cmd.Flags().StringVar(&templatePattern, "template-pattern", "", "only mutate templates whose name matches this regular expression")
	cmd.Flags().StringVar(&runPattern, "run", "", "only run tests matching this regular expression (passed to go test -run)")
	cmd.Flags().BoolVar(&config.DryRun, "dry-run", false, "enumerate the mutants and report them without running any tests")
	cmd.Flags().BoolVarP(&config.Verbose, "verbose", "v", false, "report every mutant, not only the ones no test caught, and stream progress")
	cmd.Flags().BoolVar(&config.IncludeTests, "include-test-callers", false, "also mutate templates reached only from ExecuteTemplate calls in _test.go files")
	cmd.Flags().Uint64Var(&config.Seed, "seed", 0, "seed the values substituted for an action's operands (default: drawn and reported)")
	cmd.Flags().IntVar(&config.MaxCases, "max-cases", mutation.DefaultMaxCases, "most operand combinations one action may contribute")
	cmd.Flags().StringVar(&config.Diff, "diff", "", "only mutate templates whose text, or the type of dot they are rendered with, changed since this git revision")
	cmd.Flags().IntVar(&config.Workers, "workers", 1, "how many mutants to run at once; the tests must tolerate running beside themselves")
	cmd.Flags().String("format", "text", "output format (text or json)")

	return cmd
}
