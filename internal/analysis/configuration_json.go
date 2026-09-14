package analysis

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// A listing's configuration holds compiled patterns, which carry nothing
// JSON can write. In JSON a pattern is the text it was written as -- what
// the command line passed to --match -- so a configuration can be read
// from a file, which is how the snapshot archives in testdata hold theirs.

// filterTemplates is a pattern list as JSON holds it.
type filterTemplates []*regexp.Regexp

func (patterns filterTemplates) source() []string {
	if patterns == nil {
		return nil
	}
	written := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		written = append(written, pattern.String())
	}
	return written
}

func compileFilterTemplates(written []string) (filterTemplates, error) {
	if written == nil {
		return nil, nil
	}
	patterns := make(filterTemplates, 0, len(written))
	for _, text := range written {
		pattern, err := regexp.Compile(text)
		if err != nil {
			return nil, fmt.Errorf("FilterTemplates: %w", err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

// listingJSON is the shape both listing configurations have in JSON.
type listingJSON struct {
	TemplatesVariables []string
	FilterTemplates    []string
}

func (config TemplateCallersConfiguration) MarshalJSON() ([]byte, error) {
	return json.Marshal(listingJSON{TemplatesVariables: config.TemplatesVariables, FilterTemplates: filterTemplates(config.FilterTemplates).source()})
}

func (config *TemplateCallersConfiguration) UnmarshalJSON(data []byte) error {
	var read listingJSON
	if err := json.Unmarshal(data, &read); err != nil {
		return err
	}
	patterns, err := compileFilterTemplates(read.FilterTemplates)
	if err != nil {
		return err
	}
	config.TemplatesVariables, config.FilterTemplates = read.TemplatesVariables, patterns
	return nil
}

func (config TemplateCallsConfiguration) MarshalJSON() ([]byte, error) {
	return json.Marshal(listingJSON{TemplatesVariables: config.TemplatesVariables, FilterTemplates: filterTemplates(config.FilterTemplates).source()})
}

func (config *TemplateCallsConfiguration) UnmarshalJSON(data []byte) error {
	var read listingJSON
	if err := json.Unmarshal(data, &read); err != nil {
		return err
	}
	patterns, err := compileFilterTemplates(read.FilterTemplates)
	if err != nil {
		return err
	}
	config.TemplatesVariables, config.FilterTemplates = read.TemplatesVariables, patterns
	return nil
}
