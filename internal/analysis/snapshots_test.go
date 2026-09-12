package analysis

import "regexp"

// snapshots names the configuration each archive in testdata runs with,
// which also says which analysis runs: the configuration the command line
// in the comment parses into.
//
// TestCommandLineConfigurations in internal/cli states what command lines
// parse into, with literals like these; search for a literal to find its
// twin. Every configuration here is one a command line can produce:
// rejecting one that cannot work is the command line's job.
type snapshotCase struct {
	archive string
	config  any
}

var snapshots = []snapshotCase{
	{
		// muxt list-template-callers
		archive: "callers",
		config:  TemplateCallersConfiguration{TemplatesVariables: []string{"templates"}},
	},
	{
		// muxt list-template-callers --match=^head
		archive: "callers_match",
		config:  TemplateCallersConfiguration{TemplatesVariables: []string{"templates"}, FilterTemplates: []*regexp.Regexp{regexp.MustCompile("^head")}},
	},
	{
		// muxt list-template-calls
		archive: "calls",
		config:  TemplateCallsConfiguration{TemplatesVariables: []string{"templates"}},
	},
	{
		// muxt check -v
		archive: "check_bad_route_name",
		config:  CheckConfiguration{Verbose: true, TemplatesVariables: []string{"templates"}},
	},
	{
		// muxt check
		archive: "check_passes",
		config:  CheckConfiguration{TemplatesVariables: []string{"templates"}},
	},
	{
		// muxt check
		archive: "check_template_not_found",
		config:  CheckConfiguration{TemplatesVariables: []string{"templates"}},
	},
	{
		// muxt check
		archive: "check_unused_templates",
		config:  CheckConfiguration{TemplatesVariables: []string{"templates"}},
	},
	{
		// muxt check
		archive: "check_wrong_field",
		config:  CheckConfiguration{TemplatesVariables: []string{"templates"}},
	},
	{
		// muxt --use-receiver-type=T
		archive: "routes",
		config:  DefinitionsConfiguration{ReceiverType: "T", TemplatesVariables: []string{"templates"}},
	},
}
