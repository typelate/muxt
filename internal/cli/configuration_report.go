package cli

import (
	"bytes"
	"cmp"
	"encoding/json/v2"
	"fmt"
	"io"
	"slices"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/composite"
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
// The two groups from a muxt.json, packages["<key>"] and package_defaults, also
// have a File member holding that file's absolute path, so an editor can open
// it from any directory. Text writes that path once above both, since a reader sees the
// file first and then its entry. The groups and their settings are arrays, not
// objects, because their order is part of what the report says.
type configurationReport []settingGroup

// settingGroup is the settings that come from one source. File is the muxt.json
// the source is in, empty for flags and defaults.
type settingGroup struct {
	Source   string
	Settings []reportLine
	File     string `json:",omitzero"`
}

// reportLine is one muxt.json key and the value the run uses for it.
type reportLine struct {
	Key   string
	Value any
}

// newConfigurationReport reports every setting of config, which the entry for
// key in f produced, under the source that gave it. f is nil when no entry did.
func newConfigurationReport(f *composite.File, key string, given givenSettings, config generate.RoutesFileConfiguration) (configurationReport, error) {
	// The sources in the order a setting looks for its value: the command
	// line, the entry itself, then package_defaults. A setting none of them
	// gives is a default. Flags and an entry never configure one run, so at
	// most one of the first two is present.
	type reportSource struct {
		group settingGroup
		gives func(entryField) (bool, error)
	}
	// Apply writes into a scratch configuration; only whether the entry gave
	// the key matters here.
	givenBy := func(entry composite.Entry) func(entryField) (bool, error) {
		return func(field entryField) (bool, error) {
			return field.Apply(new(generate.RoutesFileConfiguration), entry)
		}
	}
	var sources []reportSource
	if len(given) > 0 {
		sources = append(sources, reportSource{
			group: settingGroup{Source: "flags"},
			// The current name only, until the report files a deprecated
			// spelling under the setting it spells.
			gives: func(field entryField) (bool, error) { return slices.Contains(given[field.Name], field.Name), nil },
		})
	}
	if f != nil {
		sources = append(sources,
			reportSource{group: settingGroup{Source: fmt.Sprintf("packages[%q]", key), File: f.Name}, gives: givenBy(f.Packages[key])},
			reportSource{group: settingGroup{Source: "package_defaults", File: f.Name}, gives: givenBy(f.Defaults)},
		)
	}
	defaults := settingGroup{Source: "defaults"}

	// muxt generate --help lists flags by name, so the report does too.
	fields := entryFields()
	slices.SortFunc(fields, func(a, b entryField) int { return cmp.Compare(a.Name, b.Name) })
	for _, field := range fields {
		s := reportLine{Key: muxtJSONKey(field.Name), Value: field.Value(config)}
		group := &defaults
		for i := range sources {
			given, err := sources[i].gives(field)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", sources[i].group.Source, err)
			}
			if given {
				group = &sources[i].group
				break
			}
		}
		group.Settings = append(group.Settings, s)
	}
	groups := []settingGroup{}
	for _, s := range sources {
		groups = append(groups, s.group)
	}
	groups = append(groups, defaults)
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
// The file heading of a muxt.json's groups is written once, before the first of
// them, and the groups and their settings are indented under it.
func (r configurationReport) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	buf.WriteString("Configuration:\n")
	var file string
	for _, group := range r {
		indent := "  "
		if group.File != "" {
			if group.File != file {
				fmt.Fprintf(&buf, "  %s:\n", group.File)
				file = group.File
			}
			indent = "    "
		}
		fmt.Fprintf(&buf, "%s%s:\n", indent, group.Source)
		for _, s := range group.Settings {
			value, err := json.Marshal(s.Value)
			if err != nil {
				return 0, fmt.Errorf("writing %s: %w", s.Key, err)
			}
			fmt.Fprintf(&buf, "%s  %q: %s\n", indent, s.Key, value)
		}
	}
	buf.WriteString("\n")
	return io.Copy(w, &buf)
}

// routesListing is one templates variable's routes with the configuration the
// listing ran with. Embedding the routes keeps their --format json members as
// they were; Configuration is added after them. Text prints the section once,
// before the first listing, so only that listing has printConfiguration set.
//
// Key is the packages key of the muxt.json entry when the run lists several
// entries, so a reader can tell whose listing follows; it is absent otherwise.
//
// With --format json a run over several entries writes one array holding each
// listing, since several top-level objects are not a JSON document; a run over
// one package writes the object alone.
type routesListing struct {
	*analysis.Routes
	Configuration      configurationReport
	Key                string `json:",omitzero"`
	printConfiguration bool
}

// WriteTo writes the entry's key when there is one and the Configuration
// section, when the listing is the first of its package, then the routes.
func (l routesListing) WriteTo(w io.Writer) (int64, error) {
	var n int64
	if l.printConfiguration && l.Key != "" {
		written, err := fmt.Fprintln(w, l.Key)
		n += int64(written)
		if err != nil {
			return n, err
		}
	}
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
