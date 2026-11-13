package cli

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
)

// entryField ties one muxt.json key to the flag and configuration field it
// sets. Keeping the mapping in one table means a new key is one row, not a new
// branch in entryConfiguration.
type entryField struct {
	// Name is the flag the key stands for, which finalizeGenerate asks about.
	Name string
	// Apply sets the field from e and reports whether e gave the key.
	Apply func(c *generate.RoutesFileConfiguration, e composite.Entry) (given bool, err error)
	// Value returns what c holds for the key, in the form muxt.json writes it.
	Value func(c generate.RoutesFileConfiguration) any
}

// entryValue is the row for a key whose entry field is a pointer and whose
// configuration field, which field returns, takes the value as it is.
func entryValue[T any](name string, get func(composite.Entry) *T, field func(*generate.RoutesFileConfiguration) *T) entryField {
	return entryField{
		Name: name,
		Apply: func(c *generate.RoutesFileConfiguration, e composite.Entry) (bool, error) {
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
			Apply: func(c *generate.RoutesFileConfiguration, e composite.Entry) (bool, error) {
				if e.UseTemplatesVariable == nil {
					return false, nil
				}
				c.TemplatesVariables = e.UseTemplatesVariable
				return true, nil
			},
			Value: func(c generate.RoutesFileConfiguration) any { return c.TemplatesVariables },
		},
		entryValue(useReceiverType, func(e composite.Entry) *string { return e.UseReceiverType }, func(c *generate.RoutesFileConfiguration) *string { return &c.ReceiverType }),
		entryValue(useReceiverTypePackage, func(e composite.Entry) *string { return e.UseReceiverTypePackage }, func(c *generate.RoutesFileConfiguration) *string { return &c.ReceiverPackage }),
		entryValue(outputFile, func(e composite.Entry) *string { return e.OutputFile }, func(c *generate.RoutesFileConfiguration) *string { return &c.OutputFileName }),
		entryValue(outputReceiverInterface, func(e composite.Entry) *string { return e.OutputReceiverInterface }, func(c *generate.RoutesFileConfiguration) *string { return &c.ReceiverInterface }),
		entryValue(outputRoutesFunc, func(e composite.Entry) *string { return e.OutputRoutesFunc }, func(c *generate.RoutesFileConfiguration) *string { return &c.RoutesFunction }),
		entryValue(outputTemplateDataType, func(e composite.Entry) *string { return e.OutputTemplateDataType }, func(c *generate.RoutesFileConfiguration) *string { return &c.TemplateDataType }),
		entryValue(outputSSETemplateDataType, func(e composite.Entry) *string { return e.OutputSSETemplateDataType }, func(c *generate.RoutesFileConfiguration) *string { return &c.SSETemplateDataType }),
		entryValue(outputTemplateRoutePathsType, func(e composite.Entry) *string { return e.OutputTemplateRoutePathsType }, func(c *generate.RoutesFileConfiguration) *string { return &c.TemplateRoutePathsTypeName }),
		entryValue(outputTemplateRouteType, func(e composite.Entry) *string { return e.OutputTemplateRouteType }, func(c *generate.RoutesFileConfiguration) *string { return &c.TemplateRouteTypeName }),
		entryValue(outputTemplateRouteBuilderType, func(e composite.Entry) *string { return e.OutputTemplateRouteBuilderType }, func(c *generate.RoutesFileConfiguration) *string { return &c.TemplateRouteBuilderTypeName }),
		entryValue(outputRoutesFuncWithLoggerParam, func(e composite.Entry) *bool { return e.OutputRoutesFuncWithLoggerParam }, func(c *generate.RoutesFileConfiguration) *bool { return &c.Logger }),
		entryValue(outputRoutesFuncWithPathPrefix, func(e composite.Entry) *bool { return e.OutputRoutesFuncWithPathPrefixParam }, func(c *generate.RoutesFileConfiguration) *bool { return &c.PathPrefix }),
		entryValue(outputRoutesFuncWithMiddlewareParam, func(e composite.Entry) *bool { return e.OutputRoutesFuncWithMiddlewareParam }, func(c *generate.RoutesFileConfiguration) *bool { return &c.Middleware }),
		entryValue(outputMultipleFiles, func(e composite.Entry) *bool { return e.OutputMultipleFiles }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputMultipleFiles }),
		entryValue(outputHTMX, func(e composite.Entry) *bool { return e.OutputHTMX }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputHTMX }),
		entryValue(outputDatastar, func(e composite.Entry) *bool { return e.OutputDatastar }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputDatastar }),
		entryValue(outputExportedDefaultIdentifiers, func(e composite.Entry) *bool { return e.OutputExportedDefaultIdentifiers }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputExportedDefaultIdentifiers }),
		entryValue(outputMuxtVersion, func(e composite.Entry) *bool { return e.OutputMuxtVersion }, func(c *generate.RoutesFileConfiguration) *bool { return &c.OutputMuxtVersion }),
		{
			Name: outputMultipartMaxMemory,
			Apply: func(c *generate.RoutesFileConfiguration, e composite.Entry) (bool, error) {
				if e.OutputMultipartMaxMemory == nil {
					return false, nil
				}
				// The flag's own parser turns "64MB" into bytes, so the entry and
				// the command line accept the same values.
				if err := (byteSize{p: &c.MultipartMaxMemory}).Set(*e.OutputMultipartMaxMemory); err != nil {
					return false, fmt.Errorf("%s: %w", muxtJSONKey(outputMultipartMaxMemory), err)
				}
				return true, nil
			},
			// The flag prints the size the run uses, 32 MiB when none was set,
			// in a form its parser reads back.
			Value: func(c generate.RoutesFileConfiguration) any { return byteSize{p: &c.MultipartMaxMemory}.String() },
		},
		entryValue(inputStaticAssetsDirectory, func(e composite.Entry) *string { return e.InputStaticAssetsDirectory }, func(c *generate.RoutesFileConfiguration) *string { return &c.StaticAssetsDirectory }),
		entryValue(outputStaticAssetsPath, func(e composite.Entry) *string { return e.OutputStaticAssetsPath }, func(c *generate.RoutesFileConfiguration) *string { return &c.StaticAssetsPath }),
	}
}

// entryConfiguration maps the muxt.json entry for a directory onto the flag
// defaults, then runs finalizeGenerate, so an entry meets the rules a command
// line meets.
func entryConfiguration(e composite.Entry, dir string, getEnv func(string) string, version func() (string, bool)) (generate.RoutesFileConfiguration, error) {
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
