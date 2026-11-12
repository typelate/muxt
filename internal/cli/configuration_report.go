package cli

import (
	"bytes"
	"cmp"
	"encoding/json/v2"
	"fmt"
	"io"
	"slices"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxtjson"
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

// newConfigurationReport reports every setting of config, which the entry for
// key in f produced, under the source that gave it. f is nil when no entry did.
func newConfigurationReport(f *muxtjson.File, key string, config generate.RoutesFileConfiguration) (configurationReport, error) {
	// The sources in the order a setting looks for its value: the entry
	// itself, then package_defaults. A setting neither gives is a default.
	var groups []settingGroup
	var entries []muxtjson.Entry
	if f != nil {
		groups = append(groups,
			settingGroup{Source: fmt.Sprintf("%s packages[%q]", f.Name, key)},
			settingGroup{Source: f.Name + " package_defaults"},
		)
		entries = append(entries, f.Packages[key], f.Defaults)
	}
	groups = append(groups, settingGroup{Source: "defaults"})

	// muxt generate --help lists flags by name, so the report does too.
	fields := entryFields()
	slices.SortFunc(fields, func(a, b entryField) int { return cmp.Compare(a.Name, b.Name) })
	for _, field := range fields {
		s := setting{Key: muxtJSONKey(field.Name), Value: field.Value(config)}
		source := len(entries)
		for i, entry := range entries {
			// Apply writes into a scratch configuration; only whether the
			// entry gave the key matters here.
			given, err := field.Apply(new(generate.RoutesFileConfiguration), entry)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", groups[i].Source, err)
			}
			if given {
				source = i
				break
			}
		}
		groups[source].Settings = append(groups[source].Settings, s)
	}
	var report configurationReport
	for _, group := range groups {
		if len(group.Settings) > 0 {
			report = append(report, group)
		}
	}
	return report, nil
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
