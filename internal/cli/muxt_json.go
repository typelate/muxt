package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

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

// readMuxtJSONEntry returns the entry for key in the muxt.json of dir. found is
// false when dir has no muxt.json or the file has no entry for key.
func readMuxtJSONEntry(dir, key string) (entry muxtjson.Entry, found bool, err error) {
	f, err := muxtjson.Read(filepath.Join(dir, muxtjson.FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return muxtjson.Entry{}, false, nil
	}
	if err != nil {
		return muxtjson.Entry{}, false, fmt.Errorf("reading muxt.json entry %q: %w", key, err)
	}
	entry, found = f.Packages[key]
	return entry, found, nil
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
