package cli

import (
	"fmt"
	"go/token"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
)

func addGenerateFlags(flagSet *pflag.FlagSet, config *generate.RoutesFileConfiguration, deprecatedTemplatesVar *string) {
	registerWithDeprecated(flagSet, generateSettings(config))
	addDeprecatedTemplatesVariable(flagSet, deprecatedTemplatesVar)
	addVerboseFlagToFlagSet(flagSet, &config.Verbose)
}

func isDefaultTemplatesVariable(in *[]string) bool {
	return in != nil && len(*in) == 1 && (*in)[0] == defaultTemplatesVariableName
}

func addVerboseFlagToFlagSet(flagSet *pflag.FlagSet, out *bool) {
	flagSet.BoolVarP(out, "verbose", "v", false, "verbose log output")
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

func checkTemplatesVariables(templateVariables []string) error {
	for _, tv := range templateVariables {
		if tv != "" && !token.IsIdentifier(tv) {
			return fmt.Errorf("variable %s%s", tv, errIdentSuffix)
		}
	}
	return nil
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
