package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxtjson"
)

// givenGenerateFlags returns the sorted names of the generate flags passed.
// The root flags -C and -v do not configure a run, so they do not count.
func givenGenerateFlags(flags *pflag.FlagSet) []string {
	var given []string
	flags.Visit(func(f *pflag.Flag) {
		if f.Name != "change-directory" && f.Name != "verbose" {
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

// entryField ties one muxt.json key to the flag and configuration field it
// sets. Keeping the mapping in one table means a new key is one row, not a new
// branch in entryConfiguration.
type entryField struct {
	// Name is the flag the key stands for, which finalizeGenerate asks about.
	Name string
	// Apply sets the field from e and reports whether e gave the key.
	Apply func(c *generate.RoutesFileConfiguration, e muxtjson.Entry) (given bool, err error)
}

// entryValue is the row for a key whose entry field is a pointer and whose
// configuration field takes the value as it is.
func entryValue[T any](name string, get func(muxtjson.Entry) *T, set func(*generate.RoutesFileConfiguration, T)) entryField {
	return entryField{Name: name, Apply: func(c *generate.RoutesFileConfiguration, e muxtjson.Entry) (bool, error) {
		v := get(e)
		if v == nil {
			return false, nil
		}
		set(c, *v)
		return true, nil
	}}
}

// entryFields lists a row for each muxt.json key. It is a function so no
// package-level state holds the closures.
func entryFields() []entryField {
	return []entryField{
		{Name: useTemplatesVariable, Apply: func(c *generate.RoutesFileConfiguration, e muxtjson.Entry) (bool, error) {
			if e.UseTemplatesVariable == nil {
				return false, nil
			}
			c.TemplatesVariables = e.UseTemplatesVariable
			return true, nil
		}},
		entryValue(useReceiverType, func(e muxtjson.Entry) *string { return e.UseReceiverType }, func(c *generate.RoutesFileConfiguration, v string) { c.ReceiverType = v }),
		entryValue(useReceiverTypePackage, func(e muxtjson.Entry) *string { return e.UseReceiverTypePackage }, func(c *generate.RoutesFileConfiguration, v string) { c.ReceiverPackage = v }),
		entryValue(outputFile, func(e muxtjson.Entry) *string { return e.OutputFile }, func(c *generate.RoutesFileConfiguration, v string) { c.OutputFileName = v }),
		entryValue(outputReceiverInterface, func(e muxtjson.Entry) *string { return e.OutputReceiverInterface }, func(c *generate.RoutesFileConfiguration, v string) { c.ReceiverInterface = v }),
		entryValue(outputRoutesFunc, func(e muxtjson.Entry) *string { return e.OutputRoutesFunc }, func(c *generate.RoutesFileConfiguration, v string) { c.RoutesFunction = v }),
		entryValue(outputTemplateDataType, func(e muxtjson.Entry) *string { return e.OutputTemplateDataType }, func(c *generate.RoutesFileConfiguration, v string) { c.TemplateDataType = v }),
		entryValue(outputSSETemplateDataType, func(e muxtjson.Entry) *string { return e.OutputSSETemplateDataType }, func(c *generate.RoutesFileConfiguration, v string) { c.SSETemplateDataType = v }),
		entryValue(outputTemplateRoutePathsType, func(e muxtjson.Entry) *string { return e.OutputTemplateRoutePathsType }, func(c *generate.RoutesFileConfiguration, v string) { c.TemplateRoutePathsTypeName = v }),
		entryValue(outputTemplateRouteType, func(e muxtjson.Entry) *string { return e.OutputTemplateRouteType }, func(c *generate.RoutesFileConfiguration, v string) { c.TemplateRouteTypeName = v }),
		entryValue(outputTemplateRouteBuilderType, func(e muxtjson.Entry) *string { return e.OutputTemplateRouteBuilderType }, func(c *generate.RoutesFileConfiguration, v string) { c.TemplateRouteBuilderTypeName = v }),
		entryValue(outputRoutesFuncWithLoggerParam, func(e muxtjson.Entry) *bool { return e.OutputRoutesFuncWithLoggerParam }, func(c *generate.RoutesFileConfiguration, v bool) { c.Logger = v }),
		entryValue(outputRoutesFuncWithPathPrefix, func(e muxtjson.Entry) *bool { return e.OutputRoutesFuncWithPathPrefixParam }, func(c *generate.RoutesFileConfiguration, v bool) { c.PathPrefix = v }),
		entryValue(outputRoutesFuncWithMiddlewareParam, func(e muxtjson.Entry) *bool { return e.OutputRoutesFuncWithMiddlewareParam }, func(c *generate.RoutesFileConfiguration, v bool) { c.Middleware = v }),
		entryValue(outputMultipleFiles, func(e muxtjson.Entry) *bool { return e.OutputMultipleFiles }, func(c *generate.RoutesFileConfiguration, v bool) { c.OutputMultipleFiles = v }),
		entryValue(outputHTMX, func(e muxtjson.Entry) *bool { return e.OutputHTMX }, func(c *generate.RoutesFileConfiguration, v bool) { c.OutputHTMX = v }),
		entryValue(outputDatastar, func(e muxtjson.Entry) *bool { return e.OutputDatastar }, func(c *generate.RoutesFileConfiguration, v bool) { c.OutputDatastar = v }),
		entryValue(outputExportedDefaultIdentifiers, func(e muxtjson.Entry) *bool { return e.OutputExportedDefaultIdentifiers }, func(c *generate.RoutesFileConfiguration, v bool) { c.OutputExportedDefaultIdentifiers = v }),
		entryValue(outputMuxtVersion, func(e muxtjson.Entry) *bool { return e.OutputMuxtVersion }, func(c *generate.RoutesFileConfiguration, v bool) { c.OutputMuxtVersion = v }),
		{Name: outputMultipartMaxMemory, Apply: func(c *generate.RoutesFileConfiguration, e muxtjson.Entry) (bool, error) {
			if e.OutputMultipartMaxMemory == nil {
				return false, nil
			}
			// The flag's own parser turns "64MB" into bytes, so the entry and
			// the command line accept the same values.
			if err := (&multipartMaxMemoryFlag{cfg: c}).Set(*e.OutputMultipartMaxMemory); err != nil {
				return false, fmt.Errorf("%s: %w", muxtJSONKey(outputMultipartMaxMemory), err)
			}
			return true, nil
		}},
		entryValue(inputStaticAssetsDirectory, func(e muxtjson.Entry) *string { return e.InputStaticAssetsDirectory }, func(c *generate.RoutesFileConfiguration, v string) { c.StaticAssetsDirectory = v }),
		entryValue(outputStaticAssetsPath, func(e muxtjson.Entry) *string { return e.OutputStaticAssetsPath }, func(c *generate.RoutesFileConfiguration, v string) { c.StaticAssetsPath = v }),
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
