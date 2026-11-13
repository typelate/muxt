package cli

import (
	"github.com/typelate/muxt/internal/generate"
)

// generateSettings declares every setting muxt generate takes, bound to the
// fields of c. List order is the order the generated file header records them
// in; --help and the configuration report sort by name.
func generateSettings(c *generate.RoutesFileConfiguration) []setting {
	return []setting{
		templatesVariableSetting(&c.TemplatesVariables),
		receiverTypeSetting(&c.ReceiverType),
		receiverPackageSetting(&c.ReceiverPackage),
	}
}

// templatesVariableSetting is shared by every command that reads templates.
func templatesVariableSetting(p *[]string) setting {
	return stringsSetting(p, spec[[]string]{
		Name:       useTemplatesVariable,
		Usage:      useTemplatesVariableHelp,
		Default:    []string{defaultTemplatesVariableName},
		NonEmpty:   true,
		Deprecated: []string{deprecatedFindTemplatesVariable},
		Example:    "views",
	})
}
