package cli

import (
	"regexp"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/analysis"
)

func checkCommand(workingDirectory *string, run func(*cobra.Command, string, analysis.CheckConfiguration) error) *cobra.Command {
	var (
		config analysis.CheckConfiguration
		rt,
		deprecatedTemplatesVar string
	)

	cmd := &cobra.Command{
		Use:     checkCommandName,
		Aliases: []string{"c"},
		Short:   "Check templates for errors",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar); err != nil {
				return err
			}
			if err := checkTemplatesVariables(config.TemplatesVariables); err != nil {
				return err
			}
			cmd.SilenceUsage = true
			return run(cmd, *workingDirectory, config)
		},
	}

	addUseTemplatesVarToFlagSet(cmd.Flags(), &config.TemplatesVariables, &deprecatedTemplatesVar)
	addVerboseFlagToFlagSet(cmd.Flags(), &config.Verbose)
	addDeprecatedReceiverType(cmd.Flags(), &rt)

	return cmd
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

func listTemplateCallersCommand(wd *string, run func(*cobra.Command, string, analysis.TemplateCallersConfiguration) error) *cobra.Command {
	var (
		config                 analysis.TemplateCallersConfiguration
		deprecatedTemplatesVar string
		patterns               []string
	)

	cmd := &cobra.Command{
		Use:     listTemplateCallersCommandName,
		Aliases: []string{"callers"},
		Short:   "List template callers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar); err != nil {
				return err
			}
			cmd.SilenceUsage = true
			filters, err := compilePatterns(patterns)
			if err != nil {
				return err
			}
			config.FilterTemplates = filters
			return run(cmd, *wd, config)
		},
	}

	addUseTemplatesVarToFlagSet(cmd.Flags(), &config.TemplatesVariables, &deprecatedTemplatesVar)
	cmd.Flags().StringArrayVar(&patterns, "match", nil, "filter by template name (can specify multiple regular expressions)")
	cmd.Flags().String("format", "text", "output format (text or json)")

	return cmd
}

func listTemplateCallsCommand(wd *string, run func(*cobra.Command, string, analysis.TemplateCallsConfiguration) error) *cobra.Command {
	var (
		config                 analysis.TemplateCallsConfiguration
		patterns               []string
		deprecatedTemplatesVar string
	)

	cmd := &cobra.Command{
		Use:     listTemplateCallsCommandName,
		Aliases: []string{"calls"},
		Short:   "List template calls",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar); err != nil {
				return err
			}
			cmd.SilenceUsage = true
			filters, err := compilePatterns(patterns)
			if err != nil {
				return err
			}
			config.FilterTemplates = filters
			return run(cmd, *wd, config)
		},
	}

	addUseTemplatesVarToFlagSet(cmd.Flags(), &config.TemplatesVariables, &deprecatedTemplatesVar)
	cmd.Flags().StringArrayVar(&patterns, "match", nil, "filter by template name (can specify multiple regular expressions)")
	cmd.Flags().String("format", "text", "output format (text or json)")

	return cmd
}
