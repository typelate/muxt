package mutation

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// A run's configuration holds compiled patterns, which carry nothing JSON
// can write. In JSON a pattern is the text it was written as -- what the
// command line passed to --template-pattern or --run -- so a configuration
// can be read from a file, which is how the snapshot archives in testdata
// hold the one they plan with.

// configurationJSON is Configuration as JSON holds it.
type configurationJSON struct {
	TemplatesVariables []string
	TemplatePattern    string
	Run                string
	Packages           []string
	DryRun             bool
	Verbose            bool
	IncludeTests       bool
	Seed               uint64
	SeedSet            bool
	MaxCases           int
	Workers            int
	Diff               string
	GoTestArgs         []string
}

func (config Configuration) MarshalJSON() ([]byte, error) {
	written := configurationJSON{
		TemplatesVariables: config.TemplatesVariables,
		Packages:           config.Packages,
		DryRun:             config.DryRun,
		Verbose:            config.Verbose,
		IncludeTests:       config.IncludeTests,
		Seed:               config.Seed,
		SeedSet:            config.SeedSet,
		MaxCases:           config.MaxCases,
		Workers:            config.Workers,
		Diff:               config.Diff,
		GoTestArgs:         config.GoTestArgs,
	}
	if config.TemplatePattern != nil {
		written.TemplatePattern = config.TemplatePattern.String()
	}
	if config.Run != nil {
		written.Run = config.Run.String()
	}
	return json.Marshal(written)
}

func (config *Configuration) UnmarshalJSON(data []byte) error {
	var read configurationJSON
	if err := json.Unmarshal(data, &read); err != nil {
		return err
	}
	templatePattern, err := compilePattern("TemplatePattern", read.TemplatePattern)
	if err != nil {
		return err
	}
	run, err := compilePattern("Run", read.Run)
	if err != nil {
		return err
	}
	*config = Configuration{
		TemplatesVariables: read.TemplatesVariables,
		TemplatePattern:    templatePattern,
		Run:                run,
		Packages:           read.Packages,
		DryRun:             read.DryRun,
		Verbose:            read.Verbose,
		IncludeTests:       read.IncludeTests,
		Seed:               read.Seed,
		SeedSet:            read.SeedSet,
		MaxCases:           read.MaxCases,
		Workers:            read.Workers,
		Diff:               read.Diff,
		GoTestArgs:         read.GoTestArgs,
	}
	return nil
}

// compilePattern reads a pattern as the command line wrote it. An empty
// one is no pattern, which is how a run that narrows nothing reads.
func compilePattern(field, text string) (*regexp.Regexp, error) {
	if text == "" {
		return nil, nil
	}
	pattern, err := regexp.Compile(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	return pattern, nil
}
