package cli

import (
	_ "embed"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/mutation"
)

const (
	generateCommandName            = "generate"
	versionCommandName             = "version"
	checkCommandName               = "check"
	listTemplateCallersCommandName = "list-template-callers"
	listTemplateCallsCommandName   = "list-template-calls"
	exploreModuleCommandName       = "explore-module"
	generateFakeServerCommandName  = "generate-fake-server"
	testTemplateMutationsName      = "test-template-mutations"
	fmtCommandName                 = "fmt"
)

func Commands(wd string, args []string, getEnv func(string) string, stdin io.Reader, stdout, stderr io.Writer) error {
	return commands(wd, args, getEnv, cliVersion, stdin, stdout, stderr, runners{
		routes:    routesRunner(loadFromGoCommand),
		check:     runCheck,
		generate:  generateRunner(loadFromGoCommand),
		callers:   runTemplateCallers,
		calls:     runTemplateCalls,
		mutations: runTemplateMutations,
		format:    runFormatCommand,
		load:      loadFromGoCommand,
	})
}

// runners are what each command does once its flags have become a
// configuration.
//
// Parsing the flags, applying their defaults and rejecting what cannot
// work is the command line's job, and for these seven commands it is decided
// before a package is loaded; --format is checked then too, and read again
// when a result is written. Commands runs the real runners; a test runs ones that record the
// configuration, which is how what a command line means is stated without
// loading anything. generate-fake-server and explore-module load packages
// in their own RunE and have no runner yet.
type runners struct {
	routes    func(cmd *cobra.Command, jobs []routesJob) error
	check     func(cmd *cobra.Command, wd string, config analysis.CheckConfiguration) error
	generate  func(cmd *cobra.Command, jobs []packageJob) error
	callers   func(cmd *cobra.Command, wd string, config analysis.TemplateCallersConfiguration) error
	calls     func(cmd *cobra.Command, wd string, config analysis.TemplateCallsConfiguration) error
	mutations func(cmd *cobra.Command, wd string, config mutation.Configuration) error
	format    func(cmd *cobra.Command, wd string, config formatConfiguration) error

	// load is how generate def asks for the package it adds a template to. A
	// nil load skips the checks that need the package, so a test states what a
	// command line writes without running the go command.
	load packageLoader
}

func commands(wd string, args []string, getEnv func(string) string, version func() (string, bool), stdin io.Reader, stdout, stderr io.Writer, run runners) error {
	var changeDir string
	workingDirectory := &wd

	var (
		rootCommandConfig      analysis.DefinitionsConfiguration
		deprecatedTemplatesVar string
	)
	rootCmd := &cobra.Command{
		Use:   "muxt [command]",
		Short: `Generate HTTP Endpoints from HTML Templates`,
		Long: `muxt generates net/http handlers from Go HTML templates. A template whose name is a route, such as "GET /articles/{id} Article(ctx, id)", becomes a handler that calls Article on your receiver and renders the template with the result.

With no command, muxt lists the routes of the package in the working directory.

The usual order:
  muxt generate                  write template_routes.go from the templates
  muxt check                     type-check each template against its data
  muxt fmt -w .                  re-indent the templates in place
  muxt test-template-mutations   find template behavior no test checks

The documentation for this version is in the binary:
  muxt documentation                            list every page and example file
  muxt documentation docs/README.md             where to start
  muxt documentation docs/reference/cli.md      print a page
  muxt documentation --query "status code"      search every page`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if changeDir == "" {
				return nil
			}
			var newWd string
			if filepath.IsAbs(changeDir) {
				newWd = changeDir
			} else {
				cd, err := filepath.Abs(filepath.Join(wd, changeDir))
				if err != nil {
					return err
				}
				newWd = cd
			}
			*workingDirectory = newWd
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := fixTemplateVariables(&rootCommandConfig.TemplatesVariables, deprecatedTemplatesVar); err != nil {
				return err
			}
			if err := checkFormat(cmd); err != nil {
				return err
			}
			jobs, err := routesJobs(*workingDirectory, cmd.Flags(), rootCommandConfig, getEnv)
			if err != nil {
				return err
			}
			return run.routes(cmd, jobs)
		},
	}
	rootCmd.PersistentFlags().StringVarP(&changeDir, "change-directory", "C", "", "change the working directory")

	addUseTemplatesVarToFlagSet(rootCmd.Flags(), &rootCommandConfig.TemplatesVariables, &deprecatedTemplatesVar)
	addUseReceiverTypeVarToFlagSet(rootCmd.Flags(), &rootCommandConfig.ReceiverType)
	addUseReceiverTypePackageVarToFlagSet(rootCmd.Flags(), &rootCommandConfig.ReceiverPackage)
	addVerboseFlagToFlagSet(rootCmd.Flags(), &rootCommandConfig.Verbose)
	rootCmd.Flags().String("format", "text", "output format (text or json)")

	rootCmd.Flags().Bool("version", false, "print the version, as the version command does")

	rootCmd.SetArgs(versionArgs(args))
	rootCmd.SetIn(stdin)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)

	rootCmd.AddCommand(
		generateCommand(workingDirectory, getEnv, version, run.generate, run.load),
		versionCommand(version),
		checkCommand(workingDirectory, getEnv, run.check),
		formatCommand(workingDirectory, run.format),
		listTemplateCallersCommand(workingDirectory, getEnv, run.callers),
		listTemplateCallsCommand(workingDirectory, getEnv, run.calls),
		exploreModuleCommand(workingDirectory),
		generateFakeServerCommand(workingDirectory),
		testTemplateMutationsCommand(workingDirectory, run.mutations),
		documentationCommand(version),
	)
	describe(rootCmd)

	// Ensure all flag sets route their output (including deprecation warnings) to stderr
	for _, cmd := range rootCmd.Commands() {
		cmd.Flags().SetOutput(stderr)
		cmd.PersistentFlags().SetOutput(stderr)
	}
	rootCmd.Flags().SetOutput(stderr)
	rootCmd.PersistentFlags().SetOutput(stderr)

	return rootCmd.Execute()
}

// versionArgs turns a command line asking for --version into the version
// command's, so both print the same thing. Whatever else the line says is
// ignored, except -v/--verbose, which the version command understands. The
// value of a --query flag is text to search for, so a --version there is
// not a request for the version.
func versionArgs(args []string) []string {
	var asked, verbose, query bool
	for _, a := range args {
		if query {
			query = false
			continue
		}
		switch a {
		case "--":
			return versionLine(asked, verbose, args)
		case "--query", "-q":
			query = true
		case "--version":
			asked = true
		case "-v", "--verbose":
			verbose = true
		}
	}
	return versionLine(asked, verbose, args)
}

func versionLine(asked, verbose bool, args []string) []string {
	if !asked {
		return args
	}
	if verbose {
		return []string{versionCommandName, "--verbose"}
	}
	return []string{versionCommandName}
}

func versionCommand(version func() (string, bool)) *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:     versionCommandName,
		Aliases: []string{"v"},
		Short:   "Print the version number",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			v, ok := version()
			if !ok {
				return fmt.Errorf("missing CLI version")
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), v); err != nil {
				return err
			}

			if verbose {
				bi, ok := debug.ReadBuildInfo()
				if ok {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "go version: %s\n", bi.GoVersion); err != nil {
						return err
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	return cmd
}

func cliVersion() (string, bool) {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" {
		return "", false
	}
	return bi.Main.Version, true
}
