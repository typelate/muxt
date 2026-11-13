package cli

import (
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
)

// newConfigurationReport reports every setting of config, which the entry for
// key in f produced, under the source that gave it. f is nil when no entry did.
func newConfigurationReport(f *composite.File, key string, given givenSettings, config generate.RoutesFileConfiguration) analysis.ConfigurationReport {
	// The sources in the order a setting looks for its value: the command
	// line, the entry itself, then package_defaults. A setting none of them
	// gives is a default. Flags and an entry never configure one run, so at
	// most one of the first two is present.
	type reportSource struct {
		group analysis.SettingGroup
		// gives reports whether the source gives the setting called name.
		gives func(name string) bool
	}
	// A key an entry holds is a setting it gives; composite dropped nulls.
	holds := func(entry composite.Entry) func(string) bool {
		return func(name string) bool {
			_, ok := entry[composite.Key(name)]
			return ok
		}
	}
	var sources []reportSource
	if len(given) > 0 {
		sources = append(sources, reportSource{
			group: analysis.SettingGroup{Source: "flags"},
			gives: given.has,
		})
	}
	if f != nil {
		sources = append(sources,
			reportSource{group: analysis.SettingGroup{Source: fmt.Sprintf("packages[%q]", key), File: f.Name}, gives: holds(f.Packages[key])},
			reportSource{group: analysis.SettingGroup{Source: "package_defaults", File: f.Name}, gives: holds(f.Defaults)},
		)
	}
	defaults := analysis.SettingGroup{Source: "defaults"}

	// muxt generate --help lists flags by name, so the report does too.
	settings := generateSettings(&config)
	slices.SortFunc(settings, func(a, b setting) int { return cmp.Compare(a.name, b.name) })
	for _, s := range settings {
		line := analysis.ReportLine{Key: composite.Key(s.name), Value: s.reportValue()}
		group := &defaults
		for i := range sources {
			if sources[i].gives(s.name) {
				group = &sources[i].group
				break
			}
		}
		group.Settings = append(group.Settings, line)
	}
	groups := []analysis.SettingGroup{}
	for _, s := range sources {
		groups = append(groups, s.group)
	}
	groups = append(groups, defaults)
	var report analysis.ConfigurationReport
	for _, group := range groups {
		if len(group.Settings) > 0 {
			report = append(report, group)
		}
	}
	return report
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
	Configuration      analysis.ConfigurationReport
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
