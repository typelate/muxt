package cli

import (
	"fmt"
	"math"

	"github.com/dustin/go-humanize"
	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
)

func addGenerateFlags(flagSet *pflag.FlagSet, config *generate.RoutesFileConfiguration, deprecatedTemplatesVar *string) {
	addUseTemplatesVarToFlagSet(flagSet, &config.TemplatesVariables, deprecatedTemplatesVar)
	addUseReceiverTypeVarToFlagSet(flagSet, &config.ReceiverType)
	adUseReceiverTypePackageVarToFlagSet(flagSet, &config.ReceiverPackage)
	addVerboseFlagToFlagSet(flagSet, &config.Verbose)

	addOutputFlagsToFlagSet(flagSet, config)
	addDeprecatedUseFlagsToFlagSet(flagSet, config)
	addDeprecatedOutputFlagsToFlagSet(flagSet, config)
}

func isDefaultTemplatesVariable(in *[]string) bool {
	return in != nil && len(*in) == 1 && (*in)[0] == defaultTemplatesVariableName
}

// addUseTemplatesVarToFlagSet was split out because it is used for a few different commands
func addUseTemplatesVarToFlagSet(flagSet *pflag.FlagSet, out *[]string, deprecated *string) {
	flagSet.StringSliceVar(out, useTemplatesVariable, []string{defaultTemplatesVariableName}, useTemplatesVariableHelp)
	flagSet.StringVar(deprecated, deprecatedTemplatesVariable, "", "DEPRECATED: use --"+useTemplatesVariable+" instead. "+useTemplatesVariableHelp)
	markDeprecated(flagSet, deprecatedTemplatesVariable, useTemplatesVariable)
}

func addUseReceiverTypeVarToFlagSet(flagSet *pflag.FlagSet, out *string) {
	flagSet.StringVar(out, useReceiverType, "", useReceiverTypeHelp)
}

func adUseReceiverTypePackageVarToFlagSet(flagSet *pflag.FlagSet, out *string) {
	flagSet.StringVar(out, useReceiverTypePackage, "", useReceiverTypePackageHelp)
}

func addOutputFlagsToFlagSet(flagSet *pflag.FlagSet, g *generate.RoutesFileConfiguration) {
	flagSet.StringVar(&g.OutputFileName, outputFile, defaultOutputFileName, outputFileHelp)
	flagSet.StringVar(&g.ReceiverInterface, outputReceiverInterface, defaultReceiverInterfaceName, outputReceiverInterfaceHelp)
	flagSet.StringVar(&g.RoutesFunction, outputRoutesFunc, defaultRoutesFunctionName, outputRoutesFuncHelp)
	flagSet.StringVar(&g.TemplateDataType, outputTemplateDataType, defaultTemplateDataTypeName, outputTemplateDataTypeHelp)
	flagSet.StringVar(&g.SSETemplateDataType, outputSSETemplateDataType, defaultSSETemplateDataTypeName, outputSSETemplateDataTypeHelp)
	flagSet.StringVar(&g.TemplateRoutePathsTypeName, outputTemplateRoutePathsType, defaultTemplateRoutePathsTypeName, outputTemplateRoutePathsTypeHelp)
	flagSet.BoolVar(&g.Logger, outputRoutesFuncWithLoggerParam, false, outputRoutesFuncWithLoggerParamHelp)
	flagSet.BoolVar(&g.PathPrefix, outputRoutesFuncWithPathPrefix, false, outputRoutesFuncWithPathPrefixHelp)
	flagSet.BoolVar(&g.Middleware, outputRoutesFuncWithMiddlewareParam, false, outputRoutesFuncWithMiddlewareParamHelp)
	flagSet.BoolVar(&g.OutputMultipleFiles, outputMultipleFiles, false, outputMultipleFilesHelp)
	flagSet.BoolVar(&g.OutputHTMX, outputHTMX, false, outputHTMXHelp)
	flagSet.BoolVar(&g.OutputDatastar, outputDatastar, false, outputDatastarHelp)
	flagSet.BoolVar(&g.OutputExportedDefaultIdentifiers, outputExportedDefaultIdentifiers, true, outputExportedDefaultIdentifiersHelp)
	flagSet.BoolVar(&g.OutputMuxtVersion, outputMuxtVersion, true, outputMuxtVersionHelp)
	flagSet.Var(&multipartMaxMemoryFlag{cfg: g}, outputMultipartMaxMemory, outputMultipartMaxMemoryHelp)
}

// multipartMaxMemoryFlag implements pflag.Value to parse human-readable byte
// sizes (e.g. "32MB", "64MiB", "1GB") into RoutesFileConfiguration.MultipartMaxMemory.
type multipartMaxMemoryFlag struct {
	cfg *generate.RoutesFileConfiguration
}

func (f *multipartMaxMemoryFlag) String() string {
	if f == nil || f.cfg == nil {
		return humanize.IBytes(uint64(generate.DefaultMultipartMaxMemory))
	}
	n := f.cfg.MultipartMaxMemory
	if n <= 0 {
		n = generate.DefaultMultipartMaxMemory
	}
	return humanize.IBytes(uint64(n))
}

func (f *multipartMaxMemoryFlag) Set(v string) error {
	n, err := humanize.ParseBytes(v)
	if err != nil {
		return fmt.Errorf("invalid byte size %q: %w", v, err)
	}
	if n == 0 {
		return fmt.Errorf("multipart max memory must be positive, got %q", v)
	}
	if n > math.MaxInt64 {
		return fmt.Errorf("multipart max memory %q exceeds int64 maximum", v)
	}
	f.cfg.MultipartMaxMemory = int64(n)
	return nil
}

func (f *multipartMaxMemoryFlag) Type() string { return "bytes" }

func addVerboseFlagToFlagSet(flagSet *pflag.FlagSet, out *bool) {
	flagSet.BoolVarP(out, "verbose", "v", false, "verbose log output")
}

func addDeprecatedUseFlagsToFlagSet(flagSet *pflag.FlagSet, g *generate.RoutesFileConfiguration) {
	addDeprecatedReceiverType(flagSet, &g.ReceiverType)
	flagSet.StringVar(&g.ReceiverPackage, deprecatedReceiverTypePackage, "", "DEPRECATED: use --"+useReceiverTypePackage+" instead. "+useReceiverTypePackageHelp)
	flagSet.StringArrayVar(&g.TemplatesVariables, deprecatedFindTemplatesVariable, []string{defaultTemplatesVariableName}, "DEPRECATED: use --"+useTemplatesVariable+" instead. "+useTemplatesVariableHelp)
	flagSet.StringVar(&g.ReceiverType, deprecatedFindReceiverType, "", "DEPRECATED: use --"+useReceiverType+" instead. "+useReceiverTypeHelp)
	flagSet.StringVar(&g.ReceiverPackage, deprecatedFindReceiverTypePackage, "", "DEPRECATED: use --"+useReceiverTypePackage+" instead. "+useReceiverTypePackageHelp)

	markDeprecated(flagSet, deprecatedReceiverTypePackage, useReceiverTypePackage)
	markDeprecated(flagSet, deprecatedFindTemplatesVariable, useTemplatesVariable)
	markDeprecated(flagSet, deprecatedFindReceiverType, useReceiverType)
	markDeprecated(flagSet, deprecatedFindReceiverTypePackage, useReceiverTypePackage)
}

func addDeprecatedReceiverType(flagSet *pflag.FlagSet, out *string) {
	flagSet.StringVar(out, deprecatedReceiverType, "", "DEPRECATED: use --"+useReceiverType+" instead. "+useReceiverTypeHelp)
	markDeprecated(flagSet, deprecatedReceiverType, useReceiverType)
}

func addDeprecatedOutputFlagsToFlagSet(flagSet *pflag.FlagSet, g *generate.RoutesFileConfiguration) {
	flagSet.StringVar(&g.ReceiverInterface, deprecatedReceiverInterface, defaultReceiverInterfaceName, "DEPRECATED: use --"+outputReceiverInterface+" instead. "+outputReceiverInterfaceHelp)
	flagSet.StringVar(&g.RoutesFunction, deprecatedRoutesFunc, defaultRoutesFunctionName, "DEPRECATED: use --"+outputRoutesFunc+" instead. "+outputRoutesFuncHelp)
	flagSet.StringVar(&g.TemplateDataType, deprecatedTemplateDataType, defaultTemplateDataTypeName, "DEPRECATED: use --"+outputTemplateDataType+" instead. "+outputTemplateDataTypeHelp)
	flagSet.StringVar(&g.TemplateRoutePathsTypeName, deprecatedTemplateRoutePathsType, defaultTemplateRoutePathsTypeName, "DEPRECATED: use --"+outputTemplateRoutePathsType+" instead. "+outputTemplateRoutePathsTypeHelp)
	flagSet.BoolVar(&g.Logger, deprecatedLogger, false, "DEPRECATED: use --"+outputRoutesFuncWithLoggerParam+" instead. "+outputRoutesFuncWithLoggerParamHelp)
	flagSet.BoolVar(&g.PathPrefix, deprecatedPathPrefix, false, "DEPRECATED: use --"+outputRoutesFuncWithPathPrefix+" instead. "+outputRoutesFuncWithPathPrefixHelp)
	flagSet.BoolVar(&g.OutputHTMX, outputHTMXHelpers, false, "DEPRECATED: use --"+outputHTMX+" instead. "+outputHTMXHelp)

	markDeprecated(flagSet, deprecatedReceiverInterface, outputReceiverInterface)
	markDeprecated(flagSet, deprecatedRoutesFunc, outputRoutesFunc)
	markDeprecated(flagSet, deprecatedTemplateDataType, outputTemplateDataType)
	markDeprecated(flagSet, deprecatedTemplateRoutePathsType, outputTemplateRoutePathsType)
	markDeprecated(flagSet, deprecatedLogger, outputRoutesFuncWithLoggerParam)
	markDeprecated(flagSet, deprecatedPathPrefix, outputRoutesFuncWithPathPrefix)
	markDeprecated(flagSet, outputHTMXHelpers, outputHTMX)
}

func markDeprecated(flagSet *pflag.FlagSet, name, replacement string) {
	if err := flagSet.MarkDeprecated(name, "use --"+replacement+" instead"); err != nil {
		panic(err)
	}
}

func fixTemplateVariables(templateVariables *[]string, deprecatedTemplatesVar string) error {
	if deprecatedTemplatesVar != "" {
		if !isDefaultTemplatesVariable(templateVariables) {
			return fmt.Errorf("deprecated flag %s not permitted along with %s", deprecatedTemplatesVariable, useTemplatesVariable)
		}
		*templateVariables = []string{deprecatedTemplatesVar}
		return nil
	}
	if len(*templateVariables) == 0 {
		*templateVariables = []string{defaultTemplatesVariableName}
		return nil
	}
	for _, tv := range *templateVariables {
		if tv == "" {
			return fmt.Errorf("--%s value must not be empty", useTemplatesVariable)
		}
	}
	return findDuplicateVariables(*templateVariables)
}

func findDuplicateVariables(in []string) error {
	seen := make(map[string]struct{}, len(in))
	for _, tv := range in {
		if _, ok := seen[tv]; ok {
			return fmt.Errorf("duplicate template variable: %s", tv)
		}
		seen[tv] = struct{}{}
	}
	return nil
}
