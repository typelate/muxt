package cli

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
)

// TestSelectJobs states the selection table for each scope: which jobs a
// directory means, from a muxt.json on disk and a bare flag set. Nothing is
// loaded, and no command runs.
func TestSelectJobs(t *testing.T) {
	const (
		contacts  = "example.com/fieldkit/contacts"
		inventory = "example.com/fieldkit/inventory"
		tasks     = "example.com/fieldkit/tasks"
		root      = "example.com/fieldkit"
	)
	const staleWithRoot = `{"packages": {"example.com/fieldkit": {}, "example.com/fieldkit/ghost": {}}}`
	settings := []setting{receiverTypeSetting(new(string))}
	writes := jobSpec{Name: "generate", Settings: settings, Scope: eachEntry, RequireEntry: true}
	lists := jobSpec{Name: "muxt", Settings: settings, Scope: eachEntry}
	reads := jobSpec{Name: "check", Settings: settings, Scope: itsEntry}
	one := jobSpec{Name: "test-template-mutations", Settings: settings, Scope: itsEntryOnly}

	for _, tc := range []struct {
		name     string
		spec     jobSpec
		muxtJSON string
		dir      string
		flag     bool

		wantKeys  []string // "" is a Fallback job
		wantGiven []string
		wantFile  bool
		wantErr   func(error) bool
	}{
		{name: "row 1 a directory that is not one reads nothing, whatever the muxt.json lists", spec: reads, muxtJSON: fieldkitEntries, dir: "missing", wantKeys: []string{""}},
		{name: "row 1 applies to a command that writes", spec: writes, muxtJSON: fieldkitEntries, dir: "missing", wantKeys: []string{""}},
		{name: "row 1 applies to a listing", spec: lists, muxtJSON: fieldkitEntries, dir: "missing", wantKeys: []string{""}},

		{name: "row 2 generate without a muxt.json", spec: writes, dir: "contacts", wantKeys: []string{""}},
		{name: "row 2 listing without a muxt.json", spec: lists, dir: ".", wantKeys: []string{""}},
		{name: "row 2 check without a muxt.json", spec: reads, dir: "contacts", wantKeys: []string{""}},

		{name: "row 3 generate", spec: writes, muxtJSON: fieldkitEntries, dir: "contacts", flag: true, wantErr: asError[*composite.ConfiguredDirectoryError]},
		{name: "row 3 listing", spec: lists, muxtJSON: fieldkitEntries, dir: "contacts", flag: true, wantErr: asError[*composite.ConfiguredDirectoryError]},
		{name: "row 3 check", spec: reads, muxtJSON: fieldkitEntries, dir: "contacts", flag: true, wantErr: asError[*composite.ConfiguredDirectoryError]},

		{name: "row 4 generate", spec: writes, muxtJSON: fieldkitEntries, dir: "extra", flag: true, wantKeys: []string{""}, wantGiven: []string{"use-receiver-type"}, wantFile: true},
		{name: "row 4 listing at the file's directory", spec: lists, muxtJSON: fieldkitEntries, dir: ".", flag: true, wantKeys: []string{""}, wantGiven: []string{"use-receiver-type"}, wantFile: true},
		{name: "row 4 check", spec: reads, muxtJSON: fieldkitEntries, dir: "extra", flag: true, wantKeys: []string{""}, wantGiven: []string{"use-receiver-type"}, wantFile: true},

		{name: "row 5 generate", spec: writes, muxtJSON: fieldkitEmpty, dir: ".", wantErr: asError[*composite.EmptyPackagesError]},
		{name: "row 5 listing", spec: lists, muxtJSON: fieldkitEmpty, dir: ".", wantKeys: []string{""}, wantFile: true},
		{name: "row 5 check", spec: reads, muxtJSON: fieldkitEmpty, dir: ".", wantKeys: []string{""}, wantFile: true},

		{name: "row 6 generate takes every entry", spec: writes, muxtJSON: fieldkitEntries, dir: ".", wantKeys: []string{contacts, inventory, tasks}, wantFile: true},
		{name: "row 6 listing takes every entry", spec: lists, muxtJSON: fieldkitEntries, dir: ".", wantKeys: []string{contacts, inventory, tasks}, wantFile: true},
		{name: "row 6 check without an entry for the file's directory", spec: reads, muxtJSON: fieldkitEntries, dir: ".", wantKeys: []string{""}, wantFile: true},
		{name: "row 6 check with an entry for the file's directory", spec: reads, muxtJSON: fieldkitWithRoot, dir: ".", wantKeys: []string{root}, wantFile: true},
		{name: "row 6 check validates only its own entry", spec: reads, muxtJSON: fieldkitStale, dir: ".", wantKeys: []string{""}, wantFile: true},
		{name: "row 6 check with an entry for the file's directory ignores a stale one", spec: reads, muxtJSON: staleWithRoot, dir: ".", wantKeys: []string{root}, wantFile: true},
		{name: "row 6 generate validates every entry", spec: writes, muxtJSON: fieldkitStale, dir: ".", wantErr: asError[*composite.MissingDirectoryError]},

		{name: "row 6 a command of one package without an entry for the file's directory", spec: one, muxtJSON: fieldkitEntries, dir: ".", wantErr: asError[*composite.SeveralPackagesError]},
		{name: "row 6 a command of one package with an entry for the file's directory", spec: one, muxtJSON: fieldkitWithRoot, dir: ".", wantKeys: []string{root}, wantFile: true},
		{name: "row 5 a command of one package with no packages", spec: one, muxtJSON: fieldkitEmpty, dir: ".", wantKeys: []string{""}, wantFile: true},
		{name: "row 4 a command of one package with a flag", spec: one, muxtJSON: fieldkitEntries, dir: ".", flag: true, wantKeys: []string{""}, wantGiven: []string{"use-receiver-type"}, wantFile: true},

		{name: "row 7 generate", spec: writes, muxtJSON: fieldkitEntries, dir: "inventory", wantKeys: []string{inventory}, wantFile: true},
		{name: "row 7 listing", spec: lists, muxtJSON: fieldkitEntries, dir: "inventory", wantKeys: []string{inventory}, wantFile: true},
		{name: "row 7 check", spec: reads, muxtJSON: fieldkitEntries, dir: "inventory", wantKeys: []string{inventory}, wantFile: true},
		{name: "row 7 generate ignores another entry that is stale", spec: writes, muxtJSON: fieldkitStale, dir: "contacts", wantKeys: []string{contacts}, wantFile: true},

		{name: "row 8 generate", spec: writes, muxtJSON: fieldkitEntries, dir: "extra", wantErr: asError[*composite.MissingEntryError]},
		{name: "row 8 listing", spec: lists, muxtJSON: fieldkitEntries, dir: "extra", wantKeys: []string{""}, wantFile: true},
		{name: "row 8 check", spec: reads, muxtJSON: fieldkitEntries, dir: "extra", wantKeys: []string{""}, wantFile: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := newFieldkit(t, tc.muxtJSON)
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.String("use-receiver-type", "", "")
			flags.Bool("verbose", false, "")
			if tc.flag {
				require.NoError(t, flags.Set("use-receiver-type", "Server"))
			}
			spec := tc.spec
			spec.Fallback = func(string) (generate.RoutesFileConfiguration, error) {
				return generate.RoutesFileConfiguration{ReceiverType: "fallback"}, nil
			}
			sel, err := selectJobs(spec, filepath.Join(base, tc.dir), flags, func(string) string { return "" })
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, tc.wantErr(err), "selectJobs(%s) error = %v", tc.name, err)
				return
			}
			require.NoError(t, err)
			var keys []string
			for _, job := range sel.Jobs {
				keys = append(keys, job.Key)
				assert.Equal(t, job.Key == "", sel.EntryFile(job) == nil, "EntryFile(%q) is nil only for a fallback job", job.Key)
			}
			assert.Equal(t, tc.wantKeys, keys, "job keys")
			assert.Equal(t, tc.wantGiven, sel.Jobs[0].Given.spellings(), "given flags")
			assert.Equal(t, tc.wantFile, sel.File != nil, "muxt.json read")
			if tc.wantKeys[0] == "" {
				assert.Equal(t, "fallback", sel.Jobs[0].Config.ReceiverType, "fallback configuration")
			}
		})
	}
}

// TestSelectJobsCopiesVerboseOntoEveryJob states that -v is a run setting no
// muxt.json holds, so every job takes it from the spec.
func TestSelectJobsCopiesVerboseOntoEveryJob(t *testing.T) {
	base := newFieldkit(t, fieldkitEntries)
	spec := jobSpec{Name: "generate", Scope: eachEntry, Verbose: true}
	sel, err := selectJobs(spec, base, nil, func(string) string { return "" })
	require.NoError(t, err)
	require.Len(t, sel.Jobs, 3)
	for _, job := range sel.Jobs {
		assert.True(t, job.Config.Verbose, "job %q Verbose", job.Key)
	}
}

// asError reports whether err is, or wraps, an E.
func asError[E error](err error) bool {
	_, ok := errors.AsType[E](err)
	return ok
}
