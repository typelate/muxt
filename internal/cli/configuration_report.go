package cli

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"io"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
)

// configurationReport is the Configuration section of the route listing: every
// generate setting, once. It is the --format json value of the listing's
// Configuration member, an array of groups, each a Source and its Settings in
// order, each a Key as muxt.json writes it and its Value:
//
//	"Configuration": [
//		{"Source": "defaults", "Settings": [{"Key": "output_file", "Value": "template_routes.go"}]}
//	]
//
// The groups and their settings are arrays, not objects, because their order
// is part of what the report says.
type configurationReport []settingGroup

// settingGroup is the settings that come from one source.
type settingGroup struct {
	Source   string
	Settings []setting
}

// setting is one muxt.json key and the value the run uses for it.
type setting struct {
	Key   string
	Value any
}

// newConfigurationReport reports every setting of config.
func newConfigurationReport(config generate.RoutesFileConfiguration) configurationReport {
	group := settingGroup{Source: "defaults"}
	for _, field := range entryFields() {
		group.Settings = append(group.Settings, setting{Key: muxtJSONKey(field.Name), Value: field.Value(config)})
	}
	return configurationReport{group}
}

// WriteTo writes the section as text: one heading per source, a line per
// setting written so it can be pasted into muxt.json, and a blank line after.
func (r configurationReport) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	buf.WriteString("Configuration:\n")
	for _, group := range r {
		fmt.Fprintf(&buf, "  %s:\n", group.Source)
		for _, s := range group.Settings {
			value, err := json.Marshal(s.Value)
			if err != nil {
				return 0, fmt.Errorf("writing %s: %w", s.Key, err)
			}
			fmt.Fprintf(&buf, "    %q: %s\n", s.Key, value)
		}
	}
	buf.WriteString("\n")
	return io.Copy(w, &buf)
}

// routesListing is one templates variable's routes with the configuration the
// listing ran with. Embedding the routes keeps their --format json members as
// they were; Configuration is added after them. Text prints the section once,
// before the first listing, so only that listing has printConfiguration set.
type routesListing struct {
	*analysis.Routes
	Configuration      configurationReport
	printConfiguration bool
}

// WriteTo writes the Configuration section when the listing is the first of
// its package, then the routes.
func (l routesListing) WriteTo(w io.Writer) (int64, error) {
	var n int64
	if l.printConfiguration {
		written, err := l.Configuration.WriteTo(w)
		n += written
		if err != nil {
			return n, err
		}
	}
	written, err := l.Routes.WriteTo(w)
	return n + written, err
}
