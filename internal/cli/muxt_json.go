package cli

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxtjson"
)

// generateFlagGiven reports whether a generate flag was passed. The root
// flags -C and -v do not configure a run, so they do not count.
func generateFlagGiven(flags *pflag.FlagSet) bool {
	given := false
	flags.Visit(func(f *pflag.Flag) {
		if f.Name != "change-directory" && f.Name != "verbose" {
			given = true
		}
	})
	return given
}

// readMuxtJSONEntry returns the key and entry for wd in the nearest muxt.json
// at or above wd. found is false when no muxt.json is found or the file has no
// entry for the key.
func readMuxtJSONEntry(wd string) (key string, entry muxtjson.Entry, found bool, err error) {
	f, found, err := muxtjson.Find(wd)
	if err != nil {
		return "", muxtjson.Entry{}, false, fmt.Errorf("finding muxt.json for %s: %w", wd, err)
	}
	if !found {
		return "", muxtjson.Entry{}, false, nil
	}
	key, err = f.KeyFor(wd)
	if err != nil {
		return "", muxtjson.Entry{}, false, fmt.Errorf("muxt.json entry for %s: %w", wd, err)
	}
	entry, found = f.Packages[key]
	return key, entry, found, nil
}

// entryConfiguration maps the muxt.json entry for a directory onto the flag
// defaults, then runs finalizeGenerate, so an entry meets the rules a command
// line meets.
func entryConfiguration(e muxtjson.Entry, dir string, getEnv func(string) string, version func() (string, bool)) (generate.RoutesFileConfiguration, error) {
	var config generate.RoutesFileConfiguration
	var deprecatedTemplatesVar string
	flags := pflag.NewFlagSet("muxt.json", pflag.ContinueOnError)
	addGenerateFlags(flags, &config, &deprecatedTemplatesVar)
	// finalizeGenerate asks the flag set which names were given, the same as
	// for a command line, so a set field counts as given.
	given := func(name string) { flags.Lookup(name).Changed = true }
	if e.UseTemplatesVariable != nil {
		config.TemplatesVariables = e.UseTemplatesVariable
		given(useTemplatesVariable)
	}
	if e.UseReceiverType != nil {
		config.ReceiverType = *e.UseReceiverType
		given(useReceiverType)
	}
	if e.UseReceiverTypePackage != nil {
		config.ReceiverPackage = *e.UseReceiverTypePackage
		given(useReceiverTypePackage)
	}
	if e.OutputFile != nil {
		config.OutputFileName = *e.OutputFile
		given(outputFile)
	}
	if e.OutputReceiverInterface != nil {
		config.ReceiverInterface = *e.OutputReceiverInterface
		given(outputReceiverInterface)
	}
	if e.OutputRoutesFunc != nil {
		config.RoutesFunction = *e.OutputRoutesFunc
		given(outputRoutesFunc)
	}
	if e.OutputTemplateDataType != nil {
		config.TemplateDataType = *e.OutputTemplateDataType
		given(outputTemplateDataType)
	}
	if e.OutputSSETemplateDataType != nil {
		config.SSETemplateDataType = *e.OutputSSETemplateDataType
		given(outputSSETemplateDataType)
	}
	if e.OutputTemplateRoutePathsType != nil {
		config.TemplateRoutePathsTypeName = *e.OutputTemplateRoutePathsType
		given(outputTemplateRoutePathsType)
	}
	if e.OutputTemplateRouteType != nil {
		config.TemplateRouteTypeName = *e.OutputTemplateRouteType
		given(outputTemplateRouteType)
	}
	if e.OutputTemplateRouteBuilderType != nil {
		config.TemplateRouteBuilderTypeName = *e.OutputTemplateRouteBuilderType
		given(outputTemplateRouteBuilderType)
	}
	if e.OutputRoutesFuncWithLoggerParam != nil {
		config.Logger = *e.OutputRoutesFuncWithLoggerParam
		given(outputRoutesFuncWithLoggerParam)
	}
	if e.OutputRoutesFuncWithPathPrefixParam != nil {
		config.PathPrefix = *e.OutputRoutesFuncWithPathPrefixParam
		given(outputRoutesFuncWithPathPrefix)
	}
	if e.OutputRoutesFuncWithMiddlewareParam != nil {
		config.Middleware = *e.OutputRoutesFuncWithMiddlewareParam
		given(outputRoutesFuncWithMiddlewareParam)
	}
	if e.OutputMultipleFiles != nil {
		config.OutputMultipleFiles = *e.OutputMultipleFiles
		given(outputMultipleFiles)
	}
	if e.OutputHTMX != nil {
		config.OutputHTMX = *e.OutputHTMX
		given(outputHTMX)
	}
	if e.OutputDatastar != nil {
		config.OutputDatastar = *e.OutputDatastar
		given(outputDatastar)
	}
	if e.OutputExportedDefaultIdentifiers != nil {
		config.OutputExportedDefaultIdentifiers = *e.OutputExportedDefaultIdentifiers
		given(outputExportedDefaultIdentifiers)
	}
	if e.OutputMuxtVersion != nil {
		config.OutputMuxtVersion = *e.OutputMuxtVersion
		given(outputMuxtVersion)
	}
	// The flag's own parser turns "64MB" into bytes, so the entry and the
	// command line accept the same values.
	if e.OutputMultipartMaxMemory != nil {
		if err := (&multipartMaxMemoryFlag{cfg: &config}).Set(*e.OutputMultipartMaxMemory); err != nil {
			return generate.RoutesFileConfiguration{}, fmt.Errorf("%s in muxt.json: %w", outputMultipartMaxMemory, err)
		}
		given(outputMultipartMaxMemory)
	}
	if e.InputStaticAssetsDirectory != nil {
		config.StaticAssetsDirectory = *e.InputStaticAssetsDirectory
		given(inputStaticAssetsDirectory)
	}
	if e.OutputStaticAssetsPath != nil {
		config.StaticAssetsPath = *e.OutputStaticAssetsPath
		given(outputStaticAssetsPath)
	}
	if err := finalizeGenerate(&config, flags, deprecatedTemplatesVar, dir, getEnv, version); err != nil {
		return generate.RoutesFileConfiguration{}, err
	}
	return config, nil
}
