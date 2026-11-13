package cli

import (
	"cmp"
	"fmt"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/ettle/strcase"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/header"
)

// envSilenceHTTPResponseWarning silences the per-route warning about
// the response argument when set to a true value.
const envSilenceHTTPResponseWarning = "MUXT_SILENCE_WARNING_HTTP_RESPONSE_ARGUMENT"

func generateCommand(workingDirectory *string, getEnv func(string) string, version func() (string, bool), run func(*cobra.Command, []packageJob) error, loadPackages packageLoader) *cobra.Command {
	var (
		config                 generate.RoutesFileConfiguration
		deprecatedTemplatesVar string
	)

	cmd := &cobra.Command{
		Use:     generateCommandName,
		Aliases: []string{"gen", "g"},
		Short:   "Generate HTTP routes from templates",
		// Without Args cobra ignores positional arguments of a command that
		// has a parent, so a misspelled sub-command would run a full generate.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The flags parsed, so what follows rejects their values, and
			// the usage text would only bury the error that says why.
			cmd.SilenceUsage = true
			sel, err := selectJobs(generateSpec(&config, cmd.Flags(), deprecatedTemplatesVar, getEnv, version), *workingDirectory, cmd.Flags(), getEnv)
			if err != nil {
				return err
			}
			return run(cmd, sel.Jobs)
		},
	}

	addGenerateFlags(cmd.Flags(), &config, &deprecatedTemplatesVar)
	cmd.AddCommand(generateConfigCommand(workingDirectory), generatePackageCommand(workingDirectory, getEnv, version), generateDefinitionCommand(workingDirectory, getEnv, loadPackages))

	return cmd
}

// generateSpec is generate's jobSpec. Its Fallback is the configuration the
// command line gives dir, finalized like an entry, so flags and entries meet
// the same rules.
func generateSpec(config *generate.RoutesFileConfiguration, flags *pflag.FlagSet, deprecatedTemplatesVar string, getEnv func(string) string, version func() (string, bool)) jobSpec {
	settings := generateSettings(config)
	return jobSpec{
		Name:         generateCommandName,
		Settings:     settings,
		Scope:        eachEntry,
		RequireEntry: true,
		Verbose:      config.Verbose,
		Version:      version,
		Fallback: func(dir string) (generate.RoutesFileConfiguration, error) {
			env := generateEnv{Dir: dir, DeprecatedTemplatesVar: deprecatedTemplatesVar, GetEnv: getEnv, Version: version}
			if err := finalizeGenerate(config, givenFlags(flags, settings), fromCommandLine, env); err != nil {
				return generate.RoutesFileConfiguration{}, err
			}
			return *config, nil
		},
	}
}

// settingSource is where a configuration came from, which decides how a rule
// error names a setting.
type settingSource uint8

const (
	// fromCommandLine names a setting as its flag: "--output-file".
	fromCommandLine settingSource = iota + 1
	// fromMuxtJSON names a setting as its muxt.json key: "output_file".
	fromMuxtJSON
)

// flag is how an error names a setting where a flag would be written.
func (s settingSource) flag(name string) string {
	if s == fromMuxtJSON {
		return muxtJSONKey(name)
	}
	return "--" + name
}

// bare is how an error names a setting where no leading dashes are written.
func (s settingSource) bare(name string) string {
	if s == fromMuxtJSON {
		return muxtJSONKey(name)
	}
	return name
}

// RuleError is a rule a configuration breaks. Settings are the settings the
// rule is about, named as the source spells them, and Rule is what is wrong.
type RuleError struct {
	Settings []string
	Rule     string
}

func (e *RuleError) Error() string {
	if len(e.Settings) == 0 {
		return e.Rule
	}
	return strings.Join(e.Settings, " and ") + " " + e.Rule
}

// generateEnv is what finalizeGenerate reads from outside the configuration.
type generateEnv struct {
	// Dir is the package directory, where the generated file is or will be.
	Dir string
	// DeprecatedTemplatesVar is the value of the deprecated --templates-variable
	// flag, which an old command line may carry alone.
	DeprecatedTemplatesVar string
	GetEnv                 func(string) string
	// Version says what a header would record, and whether one is written.
	Version func() (string, bool)
}

// finalizeGenerate turns flag values into the configuration a run writes
// with, so every source of a configuration meets the same rules. g is what the
// source gave, because an empty value is a mistake only where it was given.
func finalizeGenerate(config *generate.RoutesFileConfiguration, g givenSettings, from settingSource, env generateEnv) error {
	if err := fixTemplateVariables(&config.TemplatesVariables, env.DeprecatedTemplatesVar); err != nil {
		return err
	}
	config.SilenceHTTPResponseWarning, _ = strconv.ParseBool(env.GetEnv(envSilenceHTTPResponseWarning))
	settings := generateSettings(config)
	if err := checkNonEmpty(settings, g, from); err != nil {
		return err
	}
	// Validation sees the names the run will generate, so a name
	// that collides with a default or derived one is caught here.
	applyDefaults(config, settings, g)
	if err := validateGenerateConfiguration(*config, from); err != nil {
		return err
	}
	if v, ok := env.Version(); ok && config.OutputMuxtVersion {
		stable, err := header.Stable(v, filepath.Join(env.Dir, config.OutputFileName))
		if err != nil {
			return err
		}
		config.MuxtVersion = stable
	}
	return nil
}

// emptyRule is what a setting that must name something breaks when it is
// given empty.
const emptyRule = "value must not be empty"

// checkNonEmpty rejects a setting that was given an empty value where it must
// name something. It runs before applyDefaults, which cannot tell an empty
// value from one never given. Every current name is checked in list order,
// then every deprecated spelling, so a line giving two empty names reports the
// current one.
func checkNonEmpty(settings []setting, g givenSettings, from settingSource) error {
	for _, s := range settings {
		if s.nonEmpty && s.empty() && slices.Contains(g[s.name], s.name) {
			return &RuleError{Settings: []string{from.flag(s.name)}, Rule: emptyRule}
		}
	}
	for _, s := range settings {
		for _, spelling := range s.deprecated {
			if s.nonEmpty && s.empty() && slices.Contains(g[s.name], spelling) {
				return &RuleError{Settings: []string{from.flag(spelling)}, Rule: emptyRule}
			}
		}
	}
	return nil
}

// validateGenerateConfiguration rejects a configuration generate cannot
// write. It is given the configuration after applyDefaults, so the names
// it compares are the ones the run would generate.
func validateGenerateConfiguration(config generate.RoutesFileConfiguration, from settingSource) error {
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
			return &RuleError{Settings: []string{from.bare(id.flag)}, Rule: identRule}
		}
	}
	if err := checkGeneratedNamesDiffer(config, from); err != nil {
		return err
	}
	if config.OutputHTMX && config.OutputDatastar {
		return &RuleError{
			Settings: []string{from.flag(outputHTMX), from.flag(outputDatastar)},
			Rule:     "are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)",
		}
	}
	if err := checkOutputFileName(config.OutputFileName, from); err != nil {
		return err
	}
	if err := checkStaticAssets(config, from); err != nil {
		return err
	}
	return checkRecordable(from, useReceiverTypePackage, config.ReceiverPackage)
}

// checkStaticAssets rejects a static assets mapping generate cannot embed
// or serve. Whether the directory exists is for generate to say, since
// the directory is relative to the package it writes into.
func checkStaticAssets(config generate.RoutesFileConfiguration, from settingSource) error {
	dir, urlPath := config.StaticAssetsDirectory, config.StaticAssetsPath
	if dir == "" && urlPath == "" {
		return nil
	}
	if dir == "" || urlPath == "" {
		return &RuleError{Settings: []string{from.flag(inputStaticAssetsDirectory), from.flag(outputStaticAssetsPath)}, Rule: "must be given together"}
	}
	pathSetting := []string{from.flag(outputStaticAssetsPath)}
	if !strings.HasPrefix(urlPath, "/") {
		return &RuleError{Settings: pathSetting, Rule: fmt.Sprintf("must start with %q: %q", "/", urlPath)}
	}
	if urlPath == "/" {
		// A file server at the root would take the 404s of the index
		// route and of every path no template registers.
		return &RuleError{Settings: pathSetting, Rule: fmt.Sprintf("must name a path below %q: %q", "/", urlPath)}
	}
	segments := strings.Split(strings.TrimSuffix(urlPath[1:], "/"), "/")
	if strings.ContainsAny(urlPath, "{}") || slices.Contains(segments, "") {
		return &RuleError{Settings: pathSetting, Rule: fmt.Sprintf("must be a literal path without wildcards or empty segments: %q", urlPath)}
	}
	trimmed := strings.TrimSuffix(dir, "/")
	if path.IsAbs(dir) || slices.Contains(strings.Split(trimmed, "/"), "..") || strings.Contains(dir, `\`) || trimmed == "." || !fs.ValidPath(trimmed) {
		return &RuleError{Settings: []string{from.flag(inputStaticAssetsDirectory)}, Rule: fmt.Sprintf("must be a slash-separated path inside the package directory: %q", dir)}
	}
	if err := checkRecordable(from, inputStaticAssetsDirectory, dir); err != nil {
		return err
	}
	return checkRecordable(from, outputStaticAssetsPath, urlPath)
}

// checkGeneratedNamesDiffer rejects two generated identifiers with one name:
// both would be declared in the package.
func checkGeneratedNamesDiffer(config generate.RoutesFileConfiguration, from settingSource) error {
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
				return &RuleError{
					Settings: []string{from.flag(a.flag), from.flag(b.flag)},
					Rule:     fmt.Sprintf("are both %s; each generated type needs its own name", a.value),
				}
			}
		}
	}
	return nil
}

// checkOutputFileName rejects an --output-file generate cannot write: the
// file is written to, and old generated files are looked for in, the
// working directory.
func checkOutputFileName(name string, from settingSource) error {
	if name == "" {
		return nil
	}
	fileSetting := []string{from.flag(outputFile)}
	if filepath.Ext(name) != ".go" {
		return &RuleError{Rule: "output filename must use .go extension"}
	}
	if filepath.Dir(name) != "." {
		return &RuleError{Settings: fileSetting, Rule: "must be a file name in the working directory: " + name}
	}
	if filepath.Base(name) == ".go" {
		return &RuleError{Settings: fileSetting, Rule: "needs a file name before the .go extension"}
	}
	return checkRecordable(from, outputFile, name)
}

// checkRecordable rejects a value the generated file's header cannot
// record: the header joins the arguments with spaces and a later run
// splits them on whitespace, so a value holding whitespace would read back
// as two arguments.
func checkRecordable(from settingSource, name, value string) error {
	if strings.ContainsFunc(value, unicode.IsSpace) {
		return &RuleError{Settings: []string{from.flag(name)}, Rule: fmt.Sprintf("must not contain whitespace: %q", value)}
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
	args = appendValueArg(args, inputStaticAssetsDirectory, config.StaticAssetsDirectory, "")
	args = appendValueArg(args, outputStaticAssetsPath, config.StaticAssetsPath, "")
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

func applyDefaults(config *generate.RoutesFileConfiguration, settings []setting, g givenSettings) {
	config.PackageName = cmp.Or(config.PackageName, defaultPackageName)

	// g files a deprecated spelling under the name it spells, so has asks
	// about either.
	if !config.OutputExportedDefaultIdentifiers {
		if !g.has(outputRoutesFunc) {
			config.RoutesFunction = strcase.ToGoCamel(defaultRoutesFunctionName)
		}
		if !g.has(outputReceiverInterface) {
			config.ReceiverInterface = strcase.ToGoCamel(defaultReceiverInterfaceName)
		}
		if !g.has(outputTemplateDataType) {
			config.TemplateDataType = strcase.ToGoCamel(defaultTemplateDataTypeName)
		}
		if !g.has(outputSSETemplateDataType) {
			config.SSETemplateDataType = strcase.ToGoCamel(defaultSSETemplateDataTypeName)
		}
		if !g.has(outputTemplateRoutePathsType) {
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
	// after the route type, unless their own settings say otherwise. Their
	// Derived functions are the only place those conventions are applied.
	derive(settings, g)
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
