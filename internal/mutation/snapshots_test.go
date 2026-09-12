package mutation

import "regexp"

// snapshots names the configuration each archive in testdata is planned
// with: the configuration the command line in the comment parses into.
//
// TestCommandLineConfigurations in internal/cli states what command lines
// parse into, with literals like these; search for a literal to find its
// twin. Every configuration here is one a command line can produce:
// rejecting one that cannot work is the command line's job.
type snapshotCase struct {
	archive string
	config  Configuration
}

var snapshots = []snapshotCase{
	{
		// muxt test-template-mutations --dry-run --seed=1 -v
		archive: "literal_template",
		config:  Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1 -v
		archive: "partials_and_trims",
		config:  Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1 -v --template-pattern=^footer$
		archive: "template_pattern",
		config:  Configuration{TemplatesVariables: []string{"templates"}, TemplatePattern: regexp.MustCompile("^footer$"), Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1 -v
		archive: "skipped_mutants",
		config:  Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1 -v --max-cases=2
		archive: "operand_budget",
		config:  Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: 2, Workers: 1},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1 -v
		archive: "delimiters",
		config:  Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1 -v --diff=main
		archive: "diff",
		config:  Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1, Diff: "main"},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1
		archive: "err_no_call_sites",
		config:  Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1
		archive: "err_no_mutations",
		config:  Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1},
	},
	{
		// muxt test-template-mutations --dry-run --seed=1 --use-templates-variable=pages
		archive: "err_missing_variable",
		config:  Configuration{TemplatesVariables: []string{"pages"}, Packages: []string{}, DryRun: true, Seed: 1, SeedSet: true, MaxCases: DefaultMaxCases, Workers: 1},
	},
}
