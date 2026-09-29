package cli

import (
	"cmp"
	"errors"
	"fmt"
	"go/token"
	"path/filepath"
	"strconv"

	"github.com/ettle/strcase"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
)

// envSilenceHTTPResponseWarning silences the per-route warning about
// the response argument when set to a true value.
const envSilenceHTTPResponseWarning = "MUXT_SILENCE_WARNING_HTTP_RESPONSE_ARGUMENT"

func generateCommand(workingDirectory *string, getEnv func(string) string, version func() (string, bool), run func(*cobra.Command, string, generate.RoutesFileConfiguration) error) *cobra.Command {
	var (
		config                 generate.RoutesFileConfiguration
		deprecatedTemplatesVar string
	)

	cmd := &cobra.Command{
		Use:     generateCommandName,
		Aliases: []string{"gen", "g"},
		Short:   "Generate HTTP routes from templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar); err != nil {
				return err
			}
			config.SilenceHTTPResponseWarning, _ = strconv.ParseBool(getEnv(envSilenceHTTPResponseWarning))
			if err := validateGenerateConfiguration(config); err != nil {
				return err
			}

			if v, ok := version(); ok && config.OutputMuxtVersion {
				config.MuxtVersion = v
			}
			applyDefaults(&config, cmd.Flags())
			cmd.SilenceUsage = true
			return run(cmd, *workingDirectory, config)
		},
	}

	addGenerateFlags(cmd.Flags(), &config, &deprecatedTemplatesVar)

	return cmd
}

func validateGenerateConfiguration(config generate.RoutesFileConfiguration) error {
	for _, tv := range config.TemplatesVariables {
		if tv != "" && !token.IsIdentifier(tv) {
			return fmt.Errorf("variable %s%s", tv, errIdentSuffix)
		}
	}
	for _, id := range []struct{ flag, value string }{
		{outputRoutesFunc, config.RoutesFunction},
		{useReceiverType, config.ReceiverType},
		{outputReceiverInterface, config.ReceiverInterface},
		{outputTemplateDataType, config.TemplateDataType},
		{outputSSETemplateDataType, config.SSETemplateDataType},
		{outputTemplateRoutePathsType, config.TemplateRoutePathsTypeName},
	} {
		if id.value != "" && !token.IsIdentifier(id.value) {
			return errors.New(id.flag + errIdentSuffix)
		}
	}
	if config.OutputHTMX && config.OutputDatastar {
		return fmt.Errorf("--%s and --%s are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)", outputHTMX, outputDatastar)
	}
	if config.OutputFileName != "" && filepath.Ext(config.OutputFileName) != ".go" {
		return errors.New("output filename must use .go extension")
	}
	return nil
}

func configToArgs(config generate.RoutesFileConfiguration) []string {
	var args []string

	if !isDefaultTemplatesVariable(&config.TemplatesVariables) {
		for _, tv := range config.TemplatesVariables {
			args = append(args, "--"+useTemplatesVariable+"="+tv)
		}
	}
	if config.ReceiverType != "" {
		args = append(args, "--"+useReceiverType+"="+config.ReceiverType)
	}
	if config.ReceiverPackage != "" {
		args = append(args, "--"+useReceiverTypePackage+"="+config.ReceiverPackage)
	}
	if config.OutputFileName != defaultOutputFileName {
		args = append(args, "--"+outputFile+"="+config.OutputFileName)
	}
	if config.ReceiverInterface != defaultReceiverInterfaceName {
		args = append(args, "--"+outputReceiverInterface+"="+config.ReceiverInterface)
	}
	if config.RoutesFunction != defaultRoutesFunctionName {
		args = append(args, "--"+outputRoutesFunc+"="+config.RoutesFunction)
	}
	if config.TemplateDataType != defaultTemplateDataTypeName {
		args = append(args, "--"+outputTemplateDataType+"="+config.TemplateDataType)
	}
	if config.SSETemplateDataType != defaultSSETemplateDataTypeName {
		args = append(args, "--"+outputSSETemplateDataType+"="+config.SSETemplateDataType)
	}
	if config.TemplateRoutePathsTypeName != defaultTemplateRoutePathsTypeName {
		args = append(args, "--"+outputTemplateRoutePathsType+"="+config.TemplateRoutePathsTypeName)
	}

	if config.Logger {
		args = append(args, "--"+outputRoutesFuncWithLoggerParam)
	}
	if config.PathPrefix {
		args = append(args, "--"+outputRoutesFuncWithPathPrefix)
	}
	if config.Middleware {
		args = append(args, "--"+outputRoutesFuncWithMiddlewareParam)
	}
	if config.OutputMultipleFiles {
		args = append(args, "--"+outputMultipleFiles)
	}
	if config.OutputHTMX {
		args = append(args, "--"+outputHTMX)
	}
	if config.OutputDatastar {
		args = append(args, "--"+outputDatastar)
	}

	if !config.OutputExportedDefaultIdentifiers {
		args = append(args, "--"+outputExportedDefaultIdentifiers+"=false")
	}

	if !config.OutputMuxtVersion {
		args = append(args, "--"+outputMuxtVersion+"=false")
	}

	if config.MultipartMaxMemory > 0 {
		args = append(args, "--"+outputMultipartMaxMemory+"="+strconv.FormatInt(config.MultipartMaxMemory, 10))
	}

	return args
}

func applyDefaults(config *generate.RoutesFileConfiguration, flagSet *pflag.FlagSet) {
	config.PackageName = cmp.Or(config.PackageName, defaultPackageName)

	if !config.OutputExportedDefaultIdentifiers {
		if !flagSet.Changed(outputRoutesFunc) {
			config.RoutesFunction = strcase.ToGoCamel(defaultRoutesFunctionName)
		}
		if !flagSet.Changed(outputReceiverInterface) {
			config.ReceiverInterface = strcase.ToGoCamel(defaultReceiverInterfaceName)
		}
		if !flagSet.Changed(outputTemplateDataType) {
			config.TemplateDataType = strcase.ToGoCamel(defaultTemplateDataTypeName)
		}
		if !flagSet.Changed(outputSSETemplateDataType) {
			config.SSETemplateDataType = strcase.ToGoCamel(defaultSSETemplateDataTypeName)
		}
		if !flagSet.Changed(outputTemplateRoutePathsType) {
			config.TemplateRoutePathsTypeName = strcase.ToGoCamel(defaultTemplateRoutePathsTypeName)
		}
	} else {
		config.RoutesFunction = cmp.Or(config.RoutesFunction, defaultRoutesFunctionName)
		config.ReceiverInterface = cmp.Or(config.ReceiverInterface, defaultReceiverInterfaceName)
		config.TemplateDataType = cmp.Or(config.TemplateDataType, defaultTemplateDataTypeName)
		config.SSETemplateDataType = cmp.Or(config.SSETemplateDataType, defaultSSETemplateDataTypeName)
		config.TemplateRoutePathsTypeName = cmp.Or(config.TemplateRoutePathsTypeName, defaultTemplateRoutePathsTypeName)
	}
}
