package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
)

func checkCommand(workingDirectory *string, getEnv func(string) string, run func(*cobra.Command, []packageJob, analysis.CheckConfiguration) error) *cobra.Command {
	var (
		config                 analysis.CheckConfiguration
		ignoredReceiverType    string
		deprecatedTemplatesVar string
	)

	settings := templatesSettings(&config.TemplatesVariables)

	cmd := &cobra.Command{
		Use:     checkCommandName + " [package-dir]",
		Aliases: []string{"c"},
		Short:   "Check templates for errors",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar, fromCommandLine); err != nil {
				return err
			}
			if err := checkTemplatesVariables(config.TemplatesVariables); err != nil {
				return err
			}
			dir, err := packageDirectory(checkCommandName, *workingDirectory, args)
			if err != nil {
				return err
			}
			sel, err := selectJobs(templatesSpec(cmd.Name(), config.TemplatesVariables, settings, getEnv), dir, cmd.Flags(), getEnv)
			if err != nil {
				return err
			}
			return run(cmd, sel.Jobs, config)
		},
	}

	registerSettings(cmd.Flags(), settings)
	addDeprecatedTemplatesVariable(cmd.Flags(), &deprecatedTemplatesVar)
	addVerboseFlagToFlagSet(cmd.Flags(), &config.Verbose)
	cmd.Flags().BoolVar(&config.AllowUnused, "allow-unused", false, "list templates nothing executes without failing; for templates rendered outside muxt")
	// v0.20.0 accepted --receiver-type here, so it still parses; check
	// takes the receiver from the generated file and never reads it.
	cmd.Flags().StringVar(&ignoredReceiverType, deprecatedReceiverType, "", "DEPRECATED and ignored: muxt check reads the receiver type from the generated routes file.")
	if err := cmd.Flags().MarkDeprecated(deprecatedReceiverType, "muxt check reads the receiver type from the generated routes file and ignores this flag"); err != nil {
		panic(err)
	}

	return cmd
}

// templatesSpec is the jobSpec of a command that reads packages and takes
// only its templates variables from the command line: check and the listings.
// In the directory of a muxt.json it reads every entry, as generate writes
// them. Its Fallback is what an empty entry gives, with the variables the
// flags named.
func templatesSpec(name string, templatesVariables []string, settings []setting, getEnv func(string) string) jobSpec {
	return jobSpec{
		Name:     name,
		Settings: settings,
		Scope:    eachEntry,
		Fallback: func(dir string) (generate.RoutesFileConfiguration, error) {
			config, err := defaultConfiguration(dir, getEnv)
			if err != nil {
				return generate.RoutesFileConfiguration{}, err
			}
			config.TemplatesVariables = templatesVariables
			return config, nil
		},
	}
}

// packageDirectory is the directory of the package a command reads: the
// one directory argument, resolved against the working directory as -C
// is, or the working directory itself when there is none. A command loads
// one package, so a second argument or a pattern is rejected.
func packageDirectory(command, wd string, args []string) (string, error) {
	switch len(args) {
	case 0:
		return wd, nil
	case 1:
	default:
		return "", fmt.Errorf("%s takes one package directory, got %d: %s", command, len(args), strings.Join(args, " "))
	}
	if strings.Contains(args[0], "...") {
		return "", fmt.Errorf("%s takes one package directory, not a pattern: %s", command, args[0])
	}
	return absoluteDir(wd, args[0]), nil
}

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	var compiled []*regexp.Regexp
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

func listTemplateCallersCommand(wd *string, getEnv func(string) string, run func(*cobra.Command, []packageJob, analysis.TemplateCallersConfiguration) error) *cobra.Command {
	var (
		config                 analysis.TemplateCallersConfiguration
		deprecatedTemplatesVar string
		patterns               []string
	)

	settings := templatesSettings(&config.TemplatesVariables)

	cmd := &cobra.Command{
		Use:     listTemplateCallersCommandName + " [package-dir]",
		Aliases: []string{"callers"},
		Short:   "List template callers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar, fromCommandLine); err != nil {
				return err
			}
			if err := checkFormat(cmd); err != nil {
				return err
			}
			filters, err := compilePatterns(patterns)
			if err != nil {
				return err
			}
			config.FilterTemplates = filters
			dir, err := packageDirectory(cmd.Name(), *wd, args)
			if err != nil {
				return err
			}
			sel, err := selectJobs(templatesSpec(cmd.Name(), config.TemplatesVariables, settings, getEnv), dir, cmd.Flags(), getEnv)
			if err != nil {
				return err
			}
			return run(cmd, sel.Jobs, config)
		},
	}

	registerSettings(cmd.Flags(), settings)
	addDeprecatedTemplatesVariable(cmd.Flags(), &deprecatedTemplatesVar)
	addVerboseFlagToFlagSet(cmd.Flags(), new(bool)) // accepted for consistency with sibling commands; the listing has nothing more to say
	cmd.Flags().StringArrayVar(&patterns, "match", nil, "filter by template name (can specify multiple regular expressions)")
	cmd.Flags().String("format", "text", "output format (text or json)")

	return cmd
}

func listTemplateCallsCommand(wd *string, getEnv func(string) string, run func(*cobra.Command, []packageJob, analysis.TemplateCallsConfiguration) error) *cobra.Command {
	var (
		config                 analysis.TemplateCallsConfiguration
		patterns               []string
		deprecatedTemplatesVar string
	)

	settings := templatesSettings(&config.TemplatesVariables)

	cmd := &cobra.Command{
		Use:     listTemplateCallsCommandName + " [package-dir]",
		Aliases: []string{"calls"},
		Short:   "List template calls",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar, fromCommandLine); err != nil {
				return err
			}
			if err := checkFormat(cmd); err != nil {
				return err
			}
			filters, err := compilePatterns(patterns)
			if err != nil {
				return err
			}
			config.FilterTemplates = filters
			dir, err := packageDirectory(cmd.Name(), *wd, args)
			if err != nil {
				return err
			}
			sel, err := selectJobs(templatesSpec(cmd.Name(), config.TemplatesVariables, settings, getEnv), dir, cmd.Flags(), getEnv)
			if err != nil {
				return err
			}
			return run(cmd, sel.Jobs, config)
		},
	}

	registerSettings(cmd.Flags(), settings)
	addDeprecatedTemplatesVariable(cmd.Flags(), &deprecatedTemplatesVar)
	addVerboseFlagToFlagSet(cmd.Flags(), new(bool)) // accepted for consistency with sibling commands; the listing has nothing more to say
	cmd.Flags().StringArrayVar(&patterns, "match", nil, "filter by template name (can specify multiple regular expressions)")
	cmd.Flags().String("format", "text", "output format (text or json)")

	return cmd
}
