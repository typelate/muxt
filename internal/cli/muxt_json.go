package cli

import (
	"fmt"
	"os"
	"slices"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxtjson"
)

// givenGenerateFlags returns the sorted names of the generate flags passed.
// The root flags -C, -v, --format and --version do not configure a run, so
// they do not count.
func givenGenerateFlags(flags *pflag.FlagSet) []string {
	var given []string
	flags.Visit(func(f *pflag.Flag) {
		if !slices.Contains([]string{"change-directory", "verbose", "format", "version"}, f.Name) {
			given = append(given, f.Name)
		}
	})
	slices.Sort(given)
	return given
}

// lookupMuxtJSONEntry returns the nearest muxt.json at or above wd, the key
// for wd in it, and that key's entry. f is nil when no muxt.json is found. found
// is false when the file has no entry under key, which is not an error here.
func lookupMuxtJSONEntry(wd string) (f *muxtjson.File, key string, entry muxtjson.Entry, found bool, err error) {
	f, found, err = muxtjson.Find(wd, deprecatedMuxtJSONKeys())
	if err != nil {
		// Find's errors already name the file, and a *muxtjson.ParseError
		// must reach the caller as it is.
		return nil, "", muxtjson.Entry{}, false, err
	}
	if !found {
		return nil, "", muxtjson.Entry{}, false, nil
	}
	key, err = f.KeyFor(wd)
	if err != nil {
		return nil, "", muxtjson.Entry{}, false, fmt.Errorf("muxt.json entry for %s: %w", wd, err)
	}
	entry, found = f.Packages[key]
	return f, key, entry, found, nil
}

// rejectConfiguredDirectory returns a *muxtjson.ConfiguredDirectoryError when
// the muxt.json for wd has an entry for it, since that entry configures the
// directory and flags would be a second source for the same fields.
func rejectConfiguredDirectory(wd string, flagNames []string) error {
	f, _, _, found, err := lookupMuxtJSONEntry(wd)
	if err != nil || !found {
		return err
	}
	return &muxtjson.ConfiguredDirectoryError{Path: f.Name, Flags: flagNames}
}

// givenEntryFlags returns the sorted names of the flags passed that a muxt.json
// entry also sets. Only these decide whether the entry or the flags configure a
// command: a flag such as --verbose or --format is not configuration, and a
// command's own flags, such as --allow-unused, do not stop the entry applying.
// The deprecated name of the templates variable flag counts as that flag.
func givenEntryFlags(flags *pflag.FlagSet) []string {
	var given []string
	flags.Visit(func(f *pflag.Flag) {
		if f.Name == deprecatedTemplatesVariable || slices.ContainsFunc(entryFields(), func(field entryField) bool { return field.Name == f.Name }) {
			given = append(given, f.Name)
		}
	})
	slices.Sort(given)
	return given
}

// entryJobFor returns the job the muxt.json entry for dir gives a command run
// in dir, and found is false when the command's own settings decide instead.
// flagNames are the entry's flags the command was given: with any, the entry
// for dir would be a second source for the same settings, so it is rejected by
// rejectConfiguredDirectory. Without them the entry is read the way generate
// reads it, so a directory below a muxt.json with no entry for it is a
// *muxtjson.MissingEntryError, as it is for generate. The command takes only
// the settings it has flags for from the job.
func entryJobFor(dir string, flagNames []string, getEnv func(string) string) (generateJob, bool, error) {
	// A path that is not a directory has no entry. The command's runner is
	// what says so, and a search for muxt.json under a file would fail first
	// with a message about muxt.json.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return generateJob{}, false, nil
	}
	if len(flagNames) > 0 {
		return generateJob{}, false, rejectConfiguredDirectory(dir, flagNames)
	}
	f, key, _, _, err := lookupMuxtJSONEntry(dir)
	if err != nil || f == nil {
		return generateJob{}, false, err
	}
	// A command that reads a package writes no header, so the version that
	// would be recorded in one is not needed.
	noVersion := func() (string, bool) { return "", false }
	_, jobs, found, err := entryJobs(dir, getEnv, noVersion)
	if err != nil || !found {
		return generateJob{}, false, err
	}
	// entryJobs returns every entry when dir is the file's own directory, so
	// the job for dir is the one whose key is dir's.
	for _, job := range jobs {
		if job.Key == key {
			return job, true, nil
		}
	}
	return generateJob{}, false, nil
}

// entryField ties one muxt.json key to the flag and configuration field it
// sets. Keeping the mapping in one table means a new key is one row, not a new
// branch in entryConfiguration.
type entryField struct {
	// Name is the flag the key stands for, which finalizeGenerate asks about.
	Name string
	// Apply sets the field from e and reports whether e gave the key.
	Apply func(c *generate.RoutesFileConfiguration, e muxtjson.Entry) (given bool, err error)
	// Value returns what c holds for the key, in the form muxt.json writes it.
	Value func(c generate.RoutesFileConfiguration) any
}

// entryValue is the row for a key whose entry field is a pointer and whose
// configuration field, which field returns, takes the value as it is.
func entryValue[T any](name string, get func(muxtjson.Entry) *T, field func(*generate.RoutesFileConfiguration) *T) entryField {
	return entryField{
		Name: name,
		Apply: func(c *generate.RoutesFileConfiguration, e muxtjson.Entry) (bool, error) {
			v := get(e)
			if v == nil {
				return false, nil
			}
			*field(c) = *v
			return true, nil
		},
		Value: func(c generate.RoutesFileConfiguration) any { return *field(&c) },
	}
}

// entryFields lists a row for each muxt.json key. It is a function so no
// package-level state holds the closures.
func entryFields() []entryField {
	return []entryField{
		{
			Name: useTemplatesVariable,
			Apply: func(c *generate.RoutesFileConfiguration, e muxtjson.Entry) (bool, error) {
				if e.UseTemplatesVariable == nil {
					return false, nil
				}
				c.TemplatesVariables = e.UseTemplatesVariable
				return true, nil
			},
			Value: func(c generate.RoutesFileConfiguration) any { return c.TemplatesVariables },
		},
		entryValue(useReceiverType, func(e muxtjson.Entry) *string { return e.UseReceiverType }, func(c *generate.RoutesFileConfiguration) *string { return &c.ReceiverType }),
		entryValue(useReceiverTypePackage, func(e muxtjson.Entry) *string { return e.UseReceiverTypePackage }, func(c *generate.RoutesFileConfiguration) *string { return &c.ReceiverPackage }),
		entryValue(outputFile, func(e muxtjson.Entry) *string { return e.OutputFile }, func(c *generate.RoutesFileConfiguration) *string { return &c.OutputFileName }),
		entryValue(outputReceiverInterface, func(e muxtjson.Entry) *string { return e.OutputReceiverInterface }, func(c *generate.RoutesFileConfiguration) *string { return &c.ReceiverInterface }),
		entryValue(outputRoutesFunc, func(e muxtjson.Entry) *string { return e.OutputRoutesFunc }, func(c *generate.RoutesFileConfiguration) *string { return &c.RoutesFunction }),
		entryValue(outputTemplateDataType, func(e muxtjson.Entry) *string { return e.OutputTemplateDataType }, func(c *generate.RoutesFileConfiguration) *string { return &c.TemplateDataType }),
		entryValue(outputSSETemplateDataType, func(e muxtjson.Entry) *string { return e.OutputSSETemplateDataType }, func(c *generate.RoutesFileConfiguration) *string { return &c.SSETemplateDataType }),
		entryValue(outputTemplateRoutePathsType, func(e muxtjson.Entry) *string { return e.OutputTemplateRoutePathsType }, func(c *generate.RoutesFileConfiguration) *string { return &c.TemplateRoutePathsTypeName }),
		entryValue(outputTemplateRouteType, func(e muxtjson.Entry) *string { return e.OutputTemplateRouteType }, func(c *generate.RoutesFileConfiguration) *string { return &c.TemplateRouteTypeName }),
		entryValue(outputTemplateRouteBuilderType, func(e muxtjson.Entry) *string { return e.OutputTemplateRouteBuilderType }, func(c *generate.RoutesFileConfiguration) *string { return &c.TemplateRouteBuilderTypeName }),
		entryValue(outputRoutesFuncWithLoggerParam, func(e muxtjson.Entry) *bool { return e.OutputRoutesFuncWithLoggerParam }, func(c *generate.RoutesFileConfiguration) *bool { return &c.Logger }),
		entryValue(outputRoutesFuncWithPathPrefix, func(e muxtjson.Entry) *bool { return e.OutputRoutesFuncWithPathPrefixParam }, func(c *generate.RoutesFileConfiguration) *bool { return &c.PathPrefix }),
		entryValue(outputRoutesFuncWithMiddlewareParam, func(e muxtjson.Entry) *bool { return e.OutputRoutesFuncWithMiddlewareParam }, func(c *generate.RoutesFileConfiguration) *bool { return &c.Middleware }),
		entryValue(outputMultipleFiles, func(e muxtjson.Entry) *bool { return e.OutputMultipleFiles }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputMultipleFiles }),
		entryValue(outputHTMX, func(e muxtjson.Entry) *bool { return e.OutputHTMX }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputHTMX }),
		entryValue(outputDatastar, func(e muxtjson.Entry) *bool { return e.OutputDatastar }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputDatastar }),
		entryValue(outputExportedDefaultIdentifiers, func(e muxtjson.Entry) *bool { return e.OutputExportedDefaultIdentifiers }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputExportedDefaultIdentifiers }),
		entryValue(outputMuxtVersion, func(e muxtjson.Entry) *bool { return e.OutputMuxtVersion }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputMuxtVersion }),
		{
			Name: outputMultipartMaxMemory,
			Apply: func(c *generate.RoutesFileConfiguration, e muxtjson.Entry) (bool, error) {
				if e.OutputMultipartMaxMemory == nil {
					return false, nil
				}
				// The flag's own parser turns "64MB" into bytes, so the entry and
				// the command line accept the same values.
				if err := (&multipartMaxMemoryFlag{cfg: c}).Set(*e.OutputMultipartMaxMemory); err != nil {
					return false, fmt.Errorf("%s: %w", muxtJSONKey(outputMultipartMaxMemory), err)
				}
				return true, nil
			},
			// The flag prints the size the run uses, 32 MiB when none was set,
			// in a form its parser reads back.
			Value: func(c generate.RoutesFileConfiguration) any { return (&multipartMaxMemoryFlag{cfg: &c}).String() },
		},
		entryValue(inputStaticAssetsDirectory, func(e muxtjson.Entry) *string { return e.InputStaticAssetsDirectory }, func(c *generate.RoutesFileConfiguration) *string { return &c.StaticAssetsDirectory }),
		entryValue(outputStaticAssetsPath, func(e muxtjson.Entry) *string { return e.OutputStaticAssetsPath }, func(c *generate.RoutesFileConfiguration) *string { return &c.StaticAssetsPath }),
	}
}

// entryConfiguration maps the muxt.json entry for a directory onto the flag
// defaults, then runs finalizeGenerate, so an entry meets the rules a command
// line meets.
func entryConfiguration(e muxtjson.Entry, dir string, getEnv func(string) string, version func() (string, bool)) (generate.RoutesFileConfiguration, error) {
	var config generate.RoutesFileConfiguration
	var deprecatedTemplatesVar string
	flags := pflag.NewFlagSet("muxt.json", pflag.ContinueOnError)
	addGenerateFlags(flags, &config, &deprecatedTemplatesVar)
	for _, field := range entryFields() {
		given, err := field.Apply(&config, e)
		if err != nil {
			return generate.RoutesFileConfiguration{}, err
		}
		// finalizeGenerate asks the flag set which names were given, the same
		// as for a command line, so a set field counts as given.
		if given {
			flags.Lookup(field.Name).Changed = true
		}
	}
	if err := finalizeGenerate(&config, flags, deprecatedTemplatesVar, dir, getEnv, version); err != nil {
		return generate.RoutesFileConfiguration{}, err
	}
	return config, nil
}
