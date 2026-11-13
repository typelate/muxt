package cli

import (
	"slices"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
)

// settingFlags returns the sorted spellings register adds to a scratch flag
// set, deprecated spellings included, less except. It asks the registration
// helpers so a new flag cannot be a setting in the command and missing here.
func settingFlags(register func(*pflag.FlagSet), except ...string) []string {
	set := pflag.NewFlagSet("settings", pflag.ContinueOnError)
	register(set)
	var names []string
	set.VisitAll(func(f *pflag.Flag) {
		if !slices.Contains(except, f.Name) {
			names = append(names, f.Name)
		}
	})
	slices.Sort(names)
	return names
}

// generateSettingFlags is every flag generate registers that configures a
// package. --verbose is a run setting, never a muxt.json key.
func generateSettingFlags() []string {
	return settingFlags(func(set *pflag.FlagSet) {
		var (
			config     generate.RoutesFileConfiguration
			deprecated string
		)
		addGenerateFlags(set, &config, &deprecated)
	}, "verbose")
}

// rootSettingFlags is what bare muxt registers that configures a package.
func rootSettingFlags() []string {
	return settingFlags(func(set *pflag.FlagSet) {
		var (
			config     analysis.DefinitionsConfiguration
			deprecated string
		)
		register(set, rootSettings(&config))
		addDeprecatedTemplatesVariable(set, &deprecated)
	})
}

// templatesSettingFlags is what the listings and check register that
// configures a package: the templates variable and its deprecated spelling.
func templatesSettingFlags() []string {
	return settingFlags(func(set *pflag.FlagSet) {
		var (
			variables  []string
			deprecated string
		)
		register(set, templatesSettings(&variables))
		addDeprecatedTemplatesVariable(set, &deprecated)
	})
}

// givenSettingFlags returns the sorted spellings in names that flags were
// given. A flag outside names, such as --verbose or a command's own flag, does
// not decide whether a muxt.json entry or the command line configures a run.
func givenSettingFlags(flags *pflag.FlagSet, names []string) []string {
	var given []string
	flags.Visit(func(f *pflag.Flag) {
		if slices.Contains(names, f.Name) {
			given = append(given, f.Name)
		}
	})
	slices.Sort(given)
	return given
}
