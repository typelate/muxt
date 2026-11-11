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
	// A flag set nothing was parsed into: no name is Changed, so the entry's
	// values are the only ones given.
	flags := pflag.NewFlagSet("muxt.json", pflag.ContinueOnError)
	addGenerateFlags(flags, &config, &deprecatedTemplatesVar)
	if e.UseReceiverType != nil {
		config.ReceiverType = *e.UseReceiverType
	}
	if e.OutputRoutesFunc != nil {
		config.RoutesFunction = *e.OutputRoutesFunc
	}
	if err := finalizeGenerate(&config, flags, deprecatedTemplatesVar, dir, getEnv, version); err != nil {
		return generate.RoutesFileConfiguration{}, err
	}
	return config, nil
}
