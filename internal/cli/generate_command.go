package cli

import (
	"cmp"
	"errors"
	"fmt"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

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
			// The flags parsed, so what follows rejects their values, and
			// the usage text would only bury the error that says why.
			cmd.SilenceUsage = true
			if err := fixTemplateVariables(&config.TemplatesVariables, deprecatedTemplatesVar); err != nil {
				return err
			}
			config.SilenceHTTPResponseWarning, _ = strconv.ParseBool(getEnv(envSilenceHTTPResponseWarning))
			if err := checkExplicitNames(cmd.Flags()); err != nil {
				return err
			}
			// Validation sees the names the run will generate, so a name
			// that collides with a default or derived one is caught here.
			applyDefaults(&config, cmd.Flags())
			if err := validateGenerateConfiguration(config); err != nil {
				return err
			}
			if v, ok := version(); ok && config.OutputMuxtVersion {
				config.MuxtVersion = v
			}
			return run(cmd, *workingDirectory, config)
		},
	}

	addGenerateFlags(cmd.Flags(), &config, &deprecatedTemplatesVar)

	return cmd
}

// explicitNameFlags are the flags that name a generated file or identifier,
// deprecated spellings included. Left unset each has a default; set, it
// must name something.
var explicitNameFlags = []string{
	outputFile,
	outputReceiverInterface,
	outputRoutesFunc,
	outputTemplateDataType,
	outputSSETemplateDataType,
	outputTemplateRoutePathsType,
	outputTemplateRouteType,
	outputTemplateRouteBuilderType,
	deprecatedReceiverInterface,
	deprecatedRoutesFunc,
	deprecatedTemplateDataType,
	deprecatedTemplateRoutePathsType,
}

// checkExplicitNames rejects a name flag that was passed with an empty
// value. It runs before applyDefaults, which cannot tell an empty value
// from one never given.
func checkExplicitNames(flagSet *pflag.FlagSet) error {
	for _, name := range explicitNameFlags {
		if flagSet.Changed(name) && flagSet.Lookup(name).Value.String() == "" {
			return fmt.Errorf("--%s value must not be empty", name)
		}
	}
	return nil
}

// validateGenerateConfiguration rejects a configuration generate cannot
// write. It is given the configuration after applyDefaults, so the names
// it compares are the ones the run would generate.
func validateGenerateConfiguration(config generate.RoutesFileConfiguration) error {
	if err := checkTemplatesVariables(config.TemplatesVariables); err != nil {
		return err
	}
	for _, id := range []struct{ flag, value string }{
		{outputRoutesFunc, config.RoutesFunction},
		{useReceiverType, config.ReceiverType},
		{outputReceiverInterface, config.ReceiverInterface},
		{outputTemplateDataType, config.TemplateDataType},
		{outputSSETemplateDataType, config.SSETemplateDataType},
		{outputTemplateRoutePathsType, config.TemplateRoutePathsTypeName},
		{outputTemplateRouteType, config.TemplateRouteTypeName},
		{outputTemplateRouteBuilderType, config.TemplateRouteBuilderTypeName},
	} {
		if id.value != "" && !token.IsIdentifier(id.value) {
			return errors.New(id.flag + errIdentSuffix)
		}
	}
	if err := checkGeneratedNamesDiffer(config); err != nil {
		return err
	}
	if config.OutputHTMX && config.OutputDatastar {
		return fmt.Errorf("--%s and --%s are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)", outputHTMX, outputDatastar)
	}
	if err := checkOutputFileName(config.OutputFileName); err != nil {
		return err
	}
	return checkRecordable(useReceiverTypePackage, config.ReceiverPackage)
}

// checkGeneratedNamesDiffer rejects two generated identifiers with one name:
// both would be declared in the package.
func checkGeneratedNamesDiffer(config generate.RoutesFileConfiguration) error {
	generated := []struct{ flag, value string }{
		{outputRoutesFunc, config.RoutesFunction},
		{outputReceiverInterface, config.ReceiverInterface},
		{outputTemplateDataType, config.TemplateDataType},
		{outputSSETemplateDataType, config.SSETemplateDataType},
		{outputTemplateRoutePathsType, config.TemplateRoutePathsTypeName},
		{outputTemplateRouteType, config.TemplateRouteTypeName},
		{outputTemplateRouteBuilderType, config.TemplateRouteBuilderTypeName},
	}
	for i, a := range generated {
		if a.value == "" {
			continue
		}
		for _, b := range generated[i+1:] {
			if a.value == b.value {
				return fmt.Errorf("--%s and --%s are both %s; each generated type needs its own name", a.flag, b.flag, a.value)
			}
		}
	}
	return nil
}

// checkOutputFileName rejects an --output-file generate cannot write: the
// file is written to, and old generated files are looked for in, the
// working directory.
func checkOutputFileName(name string) error {
	if name == "" {
		return nil
	}
	if filepath.Ext(name) != ".go" {
		return errors.New("output filename must use .go extension")
	}
	if filepath.Dir(name) != "." {
		return fmt.Errorf("--%s must be a file name in the working directory: %s", outputFile, name)
	}
	if filepath.Base(name) == ".go" {
		return fmt.Errorf("--%s needs a file name before the .go extension", outputFile)
	}
	return checkRecordable(outputFile, name)
}

// checkRecordable rejects a value the generated file's header cannot
// record: the header joins the arguments with spaces and a later run
// splits them on whitespace, so a value holding whitespace would read back
// as two arguments.
func checkRecordable(flag, value string) error {
	if strings.ContainsFunc(value, unicode.IsSpace) {
		return fmt.Errorf("--%s must not contain whitespace: %q", flag, value)
	}
	return nil
}

// routeBuilderSuffix is appended to the route type name to name the route
// builder type when --output-template-route-builder-type is not set.
const routeBuilderSuffix = "Builder"

func configToArgs(config generate.RoutesFileConfiguration) []string {
	var args []string
	if !isDefaultTemplatesVariable(&config.TemplatesVariables) {
		for _, tv := range config.TemplatesVariables {
			args = append(args, "--"+useTemplatesVariable+"="+tv)
		}
	}
	args = appendValueArg(args, useReceiverType, config.ReceiverType, "")
	args = appendValueArg(args, useReceiverTypePackage, config.ReceiverPackage, "")
	args = appendValueArg(args, outputFile, config.OutputFileName, defaultOutputFileName)
	args = appendValueArg(args, outputReceiverInterface, config.ReceiverInterface, defaultReceiverInterfaceName)
	args = appendValueArg(args, outputRoutesFunc, config.RoutesFunction, defaultRoutesFunctionName)
	args = appendValueArg(args, outputTemplateDataType, config.TemplateDataType, defaultTemplateDataTypeName)
	args = appendValueArg(args, outputSSETemplateDataType, config.SSETemplateDataType, defaultSSETemplateDataTypeName)
	args = appendValueArg(args, outputTemplateRoutePathsType, config.TemplateRoutePathsTypeName, defaultTemplateRoutePathsTypeName)
	args = appendValueArg(args, outputTemplateRouteType, config.TemplateRouteTypeName, defaultRouteTypeName(config.TemplateRoutePathsTypeName, config.OutputExportedDefaultIdentifiers))
	args = appendValueArg(args, outputTemplateRouteBuilderType, config.TemplateRouteBuilderTypeName, config.TemplateRouteTypeName+routeBuilderSuffix)
	args = appendSwitchArg(args, outputRoutesFuncWithLoggerParam, config.Logger)
	args = appendSwitchArg(args, outputRoutesFuncWithPathPrefix, config.PathPrefix)
	args = appendSwitchArg(args, outputRoutesFuncWithMiddlewareParam, config.Middleware)
	args = appendSwitchArg(args, outputMultipleFiles, config.OutputMultipleFiles)
	args = appendSwitchArg(args, outputHTMX, config.OutputHTMX)
	args = appendSwitchArg(args, outputDatastar, config.OutputDatastar)
	args = appendValueArg(args, outputExportedDefaultIdentifiers, strconv.FormatBool(config.OutputExportedDefaultIdentifiers), "true")
	args = appendValueArg(args, outputMuxtVersion, strconv.FormatBool(config.OutputMuxtVersion), "true")
	if config.MultipartMaxMemory > 0 {
		args = append(args, "--"+outputMultipartMaxMemory+"="+strconv.FormatInt(config.MultipartMaxMemory, 10))
	}
	return args
}

func appendValueArg(args []string, name, value, defaultValue string) []string {
	if value == defaultValue {
		return args
	}
	return append(args, "--"+name+"="+value)
}

func appendSwitchArg(args []string, name string, on bool) []string {
	if !on {
		return args
	}
	return append(args, "--"+name)
}

func applyDefaults(config *generate.RoutesFileConfiguration, flagSet *pflag.FlagSet) {
	config.PackageName = cmp.Or(config.PackageName, defaultPackageName)

	// changed reports whether a name was given, by its flag or the
	// deprecated spelling of it.
	changed := func(names ...string) bool {
		return slices.ContainsFunc(names, flagSet.Changed)
	}
	if !config.OutputExportedDefaultIdentifiers {
		if !changed(outputRoutesFunc, deprecatedRoutesFunc) {
			config.RoutesFunction = strcase.ToGoCamel(defaultRoutesFunctionName)
		}
		if !changed(outputReceiverInterface, deprecatedReceiverInterface) {
			config.ReceiverInterface = strcase.ToGoCamel(defaultReceiverInterfaceName)
		}
		if !changed(outputTemplateDataType, deprecatedTemplateDataType) {
			config.TemplateDataType = strcase.ToGoCamel(defaultTemplateDataTypeName)
		}
		if !changed(outputSSETemplateDataType) {
			config.SSETemplateDataType = strcase.ToGoCamel(defaultSSETemplateDataTypeName)
		}
		if !changed(outputTemplateRoutePathsType, deprecatedTemplateRoutePathsType) {
			config.TemplateRoutePathsTypeName = strcase.ToGoCamel(defaultTemplateRoutePathsTypeName)
		}
	} else {
		config.RoutesFunction = cmp.Or(config.RoutesFunction, defaultRoutesFunctionName)
		config.ReceiverInterface = cmp.Or(config.ReceiverInterface, defaultReceiverInterfaceName)
		config.TemplateDataType = cmp.Or(config.TemplateDataType, defaultTemplateDataTypeName)
		config.SSETemplateDataType = cmp.Or(config.SSETemplateDataType, defaultSSETemplateDataTypeName)
		config.TemplateRoutePathsTypeName = cmp.Or(config.TemplateRoutePathsTypeName, defaultTemplateRoutePathsTypeName)
	}

	// The route type is named after the paths type, and the route builder
	// after the route type, unless their own flags say otherwise. This is
	// the only place those conventions are applied.
	if !flagSet.Changed(outputTemplateRouteType) {
		config.TemplateRouteTypeName = defaultRouteTypeName(config.TemplateRoutePathsTypeName, config.OutputExportedDefaultIdentifiers)
	}
	if !flagSet.Changed(outputTemplateRouteBuilderType) {
		config.TemplateRouteBuilderTypeName = config.TemplateRouteTypeName + routeBuilderSuffix
	}
}

// routeTypeSuffix is appended to a route paths type name that is not the
// default to name the route type when --output-template-route-type is not
// set.
const routeTypeSuffix = "Route"

// defaultRouteTypeName is the route type a run generates when
// --output-template-route-type is not set: the default name beside the
// default paths type, and otherwise the paths type's name with Route
// appended. A package holding several route sets, each with its own
// --output-template-route-paths-type, so gets a route type per set.
func defaultRouteTypeName(pathsType string, exported bool) string {
	defaultPaths, defaultRoute := defaultTemplateRoutePathsTypeName, defaultTemplateRouteTypeName
	if !exported {
		defaultPaths, defaultRoute = strcase.ToGoCamel(defaultPaths), strcase.ToGoCamel(defaultRoute)
	}
	if pathsType == defaultPaths {
		return defaultRoute
	}
	return pathsType + routeTypeSuffix
}
