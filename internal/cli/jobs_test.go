package cli

import (
	"errors"
	"fmt"
	"os"
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
		wantKeyed bool // the run lists each package under its key
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

		{name: "row 6 generate takes every entry", spec: writes, muxtJSON: fieldkitEntries, dir: ".", wantKeys: []string{contacts, inventory, tasks}, wantFile: true, wantKeyed: true},
		{name: "row 6 listing takes every entry", spec: lists, muxtJSON: fieldkitEntries, dir: ".", wantKeys: []string{contacts, inventory, tasks}, wantFile: true, wantKeyed: true},
		{name: "row 6 listing of a file with one entry is keyed as with two", spec: lists, muxtJSON: `{"packages": {"example.com/fieldkit/contacts": {}}}`, dir: ".", wantKeys: []string{contacts}, wantFile: true, wantKeyed: true},
		{name: "row 6 generate of a file with one entry is keyed as with two", spec: writes, muxtJSON: `{"packages": {"example.com/fieldkit/contacts": {}}}`, dir: ".", wantKeys: []string{contacts}, wantFile: true, wantKeyed: true},
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

		{name: "row 7 check in the directory of a key with two sets is keyed", spec: reads, muxtJSON: twoSetsOfContacts, dir: "contacts", wantKeys: []string{contacts, contacts}, wantFile: true, wantKeyed: true},
		{name: "row 7 generate in the directory of a key with two sets is keyed", spec: writes, muxtJSON: twoSetsOfContacts, dir: "contacts", wantKeys: []string{contacts, contacts}, wantFile: true, wantKeyed: true},

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
			for _, job := range sel.Jobs {
				assert.Equal(t, tc.wantKeyed, job.Keyed, "job %q Keyed", job.Key)
			}
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

// TestSelectJobsRejectsTwoKeysForOneDirectory states that two packages keys
// whose directories are the same directory, one reached through a symbolic
// link, fail with the file, both keys and the directory, since the second
// entry's settings would otherwise win without a word.
func TestSelectJobsRejectsTwoKeysForOneDirectory(t *testing.T) {
	base := newFieldkit(t, `{"packages": {"example.com/fieldkit/contacts": {}, "example.com/fieldkit/people": {}}}`)
	if err := os.Symlink("contacts", filepath.Join(base, "people")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	spec := jobSpec{Name: "generate", Scope: eachEntry}

	_, err := selectJobs(spec, base, nil, func(string) string { return "" })

	var same *composite.SameDirectoryError
	require.ErrorAs(t, err, &same, "error %v", err)
	realBase, evalErr := filepath.EvalSymlinks(base)
	require.NoError(t, evalErr)
	assert.Equal(t, "example.com/fieldkit/people", same.Key, "key")
	assert.Equal(t, "example.com/fieldkit/contacts", same.Other, "other key")
	assert.Equal(t, filepath.Join(realBase, "contacts"), same.Dir, "directory")
	want := filepath.Join(base, composite.FileName) + `: packages["example.com/fieldkit/people"] is the same directory as packages["example.com/fieldkit/contacts"], ` + filepath.Join(realBase, "contacts") + `; keep one`
	assert.Equal(t, want, err.Error(), "message")
}

// twoSetsOfContacts is a package with a public and an admin route set, each
// with its own output file and routes function.
const twoSetsOfContacts = `{"packages": {"example.com/fieldkit/contacts": [
	{"output_file": "routes_public.go", "output_routes_func": "PublicRoutes"},
	{"output_file": "routes_admin.go", "output_routes_func": "AdminRoutes"}
]}}`

// TestSelectJobsTakesEverySetUnderAKey states that a key holding two entries
// gives one job per entry in the file's order, each knowing its position and
// how many its key holds.
func TestSelectJobsTakesEverySetUnderAKey(t *testing.T) {
	base := newFieldkit(t, twoSetsOfContacts)
	spec := jobSpec{Name: "generate", Scope: eachEntry}

	sel, err := selectJobs(spec, base, nil, func(string) string { return "" })

	require.NoError(t, err)
	var got []string
	for _, job := range sel.Jobs {
		got = append(got, fmt.Sprintf("%s %d/%d %s", job.Key, job.Set, job.Sets, job.Config.RoutesFunction))
	}
	want := []string{
		"example.com/fieldkit/contacts 0/2 PublicRoutes",
		"example.com/fieldkit/contacts 1/2 AdminRoutes",
	}
	assert.Equal(t, want, got, "jobs")
}

// TestJobNameTellsSetsOfOneKeyApart states that a job is named by its key when
// the key holds one entry, so a one-entry module prints what it always did, and
// by its key and output file when the key holds several.
func TestJobNameTellsSetsOfOneKeyApart(t *testing.T) {
	spec := jobSpec{Name: "generate", Scope: eachEntry}
	for _, tt := range []struct {
		name     string
		muxtJSON string
		want     []string
	}{
		{
			name:     "one set",
			muxtJSON: `{"packages": {"example.com/fieldkit/contacts": [{"output_file": "routes_public.go"}]}}`,
			want:     []string{"example.com/fieldkit/contacts"},
		},
		{
			name:     "several sets",
			muxtJSON: twoSetsOfContacts,
			want: []string{
				"example.com/fieldkit/contacts (routes_public.go)",
				"example.com/fieldkit/contacts (routes_admin.go)",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := newFieldkit(t, tt.muxtJSON)

			sel, err := selectJobs(spec, base, nil, func(string) string { return "" })

			require.NoError(t, err)
			var got []string
			for _, job := range sel.Jobs {
				got = append(got, job.Name())
			}
			assert.Equal(t, tt.want, got, "job names")
		})
	}
}

// TestSelectJobsNamesTheEntryAFlagRejects states that an entry whose values a
// flag rejects is named by its position under the key, so the second of two
// is not mistaken for the first.
func TestSelectJobsNamesTheEntryAFlagRejects(t *testing.T) {
	base := newFieldkit(t, `{"packages": {"example.com/fieldkit/contacts": [
		{"output_file": "routes_public.go", "output_routes_func": "PublicRoutes"},
		{"output_file": "routes_admin.go", "output_routes_func": "AdminRoutes", "output_htmx": true, "output_datastar": true}
	]}}`)
	spec := jobSpec{Name: "generate", Scope: eachEntry}

	_, err := selectJobs(spec, base, nil, func(string) string { return "" })

	var invalid *composite.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	assert.Equal(t, `packages["example.com/fieldkit/contacts"][1]`, invalid.Where, "where")
}

// TestSelectJobsRejectsTwoSetsThatShareOneOutput states that two entries of a
// key may not write one file or share one routes function, since generated
// files are owned by routes function and the later run would delete the
// earlier one's file. The error names the file, both positions and the member
// to change.
func TestSelectJobsRejectsTwoSetsThatShareOneOutput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		entries  string
		wantErr  composite.SetClashError
		wantText string
	}{
		{
			name: "output file",
			entries: `{"output_file": "routes.go", "output_routes_func": "PublicRoutes"},
				{"output_file": "routes.go", "output_routes_func": "AdminRoutes"}`,
			wantErr:  composite.SetClashError{Key: "example.com/fieldkit/contacts", Member: "output_file", Value: "routes.go", First: 0, Second: 1},
			wantText: `: packages["example.com/fieldkit/contacts"][0] and packages["example.com/fieldkit/contacts"][1] both have output_file "routes.go"; give each its own output_file`,
		},
		{
			name: "routes function",
			entries: `{"output_file": "routes_public.go", "output_routes_func": "Routes"},
				{"output_file": "routes_admin.go"},
				{"output_file": "routes_other.go", "output_routes_func": "Routes"}`,
			wantErr:  composite.SetClashError{Key: "example.com/fieldkit/contacts", Member: "output_routes_func", Value: "Routes", First: 0, Second: 2},
			wantText: `: packages["example.com/fieldkit/contacts"][0] and packages["example.com/fieldkit/contacts"][2] both have output_routes_func "Routes"; give each its own output_routes_func`,
		},
		{
			name: "routes function by default",
			entries: `{"output_file": "routes_public.go"},
				{"output_file": "routes_admin.go"}`,
			wantErr:  composite.SetClashError{Key: "example.com/fieldkit/contacts", Member: "output_routes_func", Value: "TemplateRoutes", First: 0, Second: 1},
			wantText: `: packages["example.com/fieldkit/contacts"][0] and packages["example.com/fieldkit/contacts"][1] both have output_routes_func "TemplateRoutes"; give each its own output_routes_func`,
		},
		{
			name: "output file by default",
			entries: `{"output_routes_func": "PublicRoutes"},
				{"output_routes_func": "AdminRoutes"}`,
			wantErr:  composite.SetClashError{Key: "example.com/fieldkit/contacts", Member: "output_file", Value: "template_routes.go", First: 0, Second: 1},
			wantText: `: packages["example.com/fieldkit/contacts"][0] and packages["example.com/fieldkit/contacts"][1] both have output_file "template_routes.go"; give each its own output_file`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := newFieldkit(t, `{"packages": {"example.com/fieldkit/contacts": [`+tc.entries+`]}}`)
			spec := jobSpec{Name: "generate", Scope: eachEntry}

			_, err := selectJobs(spec, base, nil, func(string) string { return "" })

			var clash *composite.SetClashError
			require.ErrorAs(t, err, &clash, "error %v", err)
			path := filepath.Join(base, composite.FileName)
			want := tc.wantErr
			want.Path = path
			assert.Equal(t, want, *clash, "error")
			assert.Equal(t, path+tc.wantText, err.Error(), "message")
		})
	}
}

// TestSelectJobsRejectsRoutePathsWithSeveralEntriesUnderAKey states that the
// route_paths hub names each package once, so a key with two entries is an
// error of its own and not a namespace clash that tells the user to rename a
// package.
func TestSelectJobsRejectsRoutePathsWithSeveralEntriesUnderAKey(t *testing.T) {
	base := newFieldkit(t, `{"route_paths": {}, "packages": {
		"example.com/fieldkit": {},
		"example.com/fieldkit/contacts": [
			{"output_file": "routes_public.go", "output_routes_func": "PublicRoutes"},
			{"output_file": "routes_admin.go", "output_routes_func": "AdminRoutes"}
		]}}`)
	spec := jobSpec{Name: "generate", Scope: eachEntry}

	_, err := selectJobs(spec, base, nil, func(string) string { return "" })

	var got *composite.RoutePathsSetsError
	require.ErrorAs(t, err, &got, "error %v", err)
	path := filepath.Join(base, composite.FileName)
	assert.Equal(t, composite.RoutePathsSetsError{Path: path, Key: "example.com/fieldkit/contacts", Sets: 2}, *got, "error")
	assert.Equal(t, path+`: route_paths takes one entry per package, and packages["example.com/fieldkit/contacts"] has 2; keep one entry or remove route_paths`, err.Error(), "message")
}

// TestSelectJobsInAPackageDirectoryTakesEverySetOfItsKey states that a run in
// the directory of a key with two entries acts on both, whatever the scope,
// and on no other key.
func TestSelectJobsInAPackageDirectoryTakesEverySetOfItsKey(t *testing.T) {
	for _, spec := range []jobSpec{
		{Name: "generate", Scope: eachEntry, RequireEntry: true},
		{Name: "check", Scope: itsEntry},
		{Name: "test-template-mutations", Scope: itsEntryOnly},
	} {
		t.Run(spec.Name, func(t *testing.T) {
			base := newFieldkit(t, `{"packages": {
				"example.com/fieldkit/inventory": {},
				"example.com/fieldkit/contacts": [
					{"output_file": "routes_public.go", "output_routes_func": "PublicRoutes"},
					{"output_file": "routes_admin.go", "output_routes_func": "AdminRoutes"}
				]}}`)

			sel, err := selectJobs(spec, filepath.Join(base, "contacts"), nil, func(string) string { return "" })

			require.NoError(t, err)
			var got []string
			for _, job := range sel.Jobs {
				got = append(got, fmt.Sprintf("%s %s", job.Key, job.Config.RoutesFunction))
			}
			want := []string{"example.com/fieldkit/contacts PublicRoutes", "example.com/fieldkit/contacts AdminRoutes"}
			assert.Equal(t, want, got, "jobs")
		})
	}
}

// TestSelectJobsRejectsTwoKeysWithSeveralEntriesForOneDirectory states that two
// keys reaching one directory fail as they do with one entry each: the error
// names the keys, not an entry, since the directory is the key's.
func TestSelectJobsRejectsTwoKeysWithSeveralEntriesForOneDirectory(t *testing.T) {
	base := newFieldkit(t, `{"packages": {
		"example.com/fieldkit/contacts": [{"output_file": "a.go", "output_routes_func": "A"}, {"output_file": "b.go", "output_routes_func": "B"}],
		"example.com/fieldkit/people": [{"output_file": "c.go", "output_routes_func": "C"}]
	}}`)
	if err := os.Symlink("contacts", filepath.Join(base, "people")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	spec := jobSpec{Name: "generate", Scope: eachEntry}

	_, err := selectJobs(spec, base, nil, func(string) string { return "" })

	var same *composite.SameDirectoryError
	require.ErrorAs(t, err, &same, "error %v", err)
	realBase, evalErr := filepath.EvalSymlinks(base)
	require.NoError(t, evalErr)
	want := filepath.Join(base, composite.FileName) + `: packages["example.com/fieldkit/people"] is the same directory as packages["example.com/fieldkit/contacts"], ` + filepath.Join(realBase, "contacts") + `; keep one`
	assert.Equal(t, want, err.Error(), "message")
}

// asError reports whether err is, or wraps, an E.
func asError[E error](err error) bool {
	_, ok := errors.AsType[E](err)
	return ok
}
