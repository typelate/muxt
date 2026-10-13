package cli

import (
	_ "embed"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
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
)

func Commands(wd string, args []string, getEnv func(string) string, stdout, stderr io.Writer) error {
	return commands(wd, args, getEnv, cliVersion, stdout, stderr, runners{
		routes:    runRoutes,
		check:     runCheck,
		generate:  runGenerate,
		callers:   runTemplateCallers,
		calls:     runTemplateCalls,
		mutations: runTemplateMutations,
	})
}

// runners are what each command does once its flags have become a
// configuration.
//
// Parsing the flags, applying their defaults and rejecting what cannot
// work is the command line's job, and for these six commands it is decided
// before a package is loaded; --format alone is still read when a result is
// written. Commands runs the real runners; a test runs ones that record the
// configuration, which is how what a command line means is stated without
// loading anything. generate-fake-server and explore-module load packages
// in their own RunE and have no runner yet.
type runners struct {
	routes    func(cmd *cobra.Command, wd string, config analysis.DefinitionsConfiguration) error
	check     func(cmd *cobra.Command, wd string, config analysis.CheckConfiguration) error
	generate  func(cmd *cobra.Command, wd string, config generate.RoutesFileConfiguration) error
	callers   func(cmd *cobra.Command, wd string, config analysis.TemplateCallersConfiguration) error
	calls     func(cmd *cobra.Command, wd string, config analysis.TemplateCallsConfiguration) error
	mutations func(cmd *cobra.Command, wd string, config mutation.Configuration) error
}

func commands(wd string, args []string, getEnv func(string) string, version func() (string, bool), stdout, stderr io.Writer, run runners) error {
	var changeDir string
	workingDirectory := &wd

	var (
		rootCommandConfig      analysis.DefinitionsConfiguration
		deprecatedTemplatesVar string
	)
	rootCmd := &cobra.Command{
		Use:   "muxt [command]",
		Short: `Generate HTTP Endpoints from HTML Templates`,
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
			return run.routes(cmd, *workingDirectory, rootCommandConfig)
		},
	}
	rootCmd.PersistentFlags().StringVarP(&changeDir, "change-directory", "C", "", "change the working directory")

	addUseTemplatesVarToFlagSet(rootCmd.Flags(), &rootCommandConfig.TemplatesVariables, &deprecatedTemplatesVar)
	addUseReceiverTypeVarToFlagSet(rootCmd.Flags(), &rootCommandConfig.ReceiverType)
	addUseReceiverTypePackageVarToFlagSet(rootCmd.Flags(), &rootCommandConfig.ReceiverPackage)
	addVerboseFlagToFlagSet(rootCmd.Flags(), &rootCommandConfig.Verbose)
	rootCmd.Flags().String("format", "text", "output format (text or json)")

	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)

	rootCmd.AddCommand(
		generateCommand(workingDirectory, getEnv, version, run.generate),
		versionCommand(version),
		checkCommand(workingDirectory, run.check),
		listTemplateCallersCommand(workingDirectory, run.callers),
		listTemplateCallsCommand(workingDirectory, run.calls),
		exploreModuleCommand(workingDirectory),
		generateFakeServerCommand(workingDirectory),
		testTemplateMutationsCommand(workingDirectory, run.mutations),
	)

	// Ensure all flag sets route their output (including deprecation warnings) to stderr
	for _, cmd := range rootCmd.Commands() {
		cmd.Flags().SetOutput(stderr)
		cmd.PersistentFlags().SetOutput(stderr)
	}
	rootCmd.Flags().SetOutput(stderr)
	rootCmd.PersistentFlags().SetOutput(stderr)

	return rootCmd.Execute()
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
