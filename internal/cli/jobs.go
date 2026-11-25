package cli

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
)

// scope is what a flagless run in the muxt.json's own directory acts on.
type scope uint8

const (
	// eachEntry is every entry, each validated before any is returned.
	eachEntry scope = iota + 1
	// itsEntry is the directory's entry, else the spec's Fallback.
	itsEntry
	// itsEntryOnly is itsEntry, except that in the muxt.json's own directory
	// without an entry it is an error when the file lists packages: the
	// command acts on one, and the directory is not one of them.
	itsEntryOnly
)

// jobSpec is what a command tells selectJobs about itself, so one function
// answers which packages and settings a directory means for every command.
type jobSpec struct {
	// Name is the command in errors: "generate", "muxt", "check", ...
	Name string
	// Settings are what the command registers that configures a package. A
	// flag that is not a spelling of one, such as --verbose or --format, never
	// decides between a muxt.json entry and the command line.
	Settings []setting
	Scope    scope
	// OnePackage words the error of an itsEntryOnly run in a muxt.json's own
	// directory that is not one of its packages.
	OnePackage onePackageWording
	// RequireEntry makes a flagless run without an entry for its directory an
	// error, for the command that writes from the file.
	RequireEntry bool
	// Verbose is a run setting, never a muxt.json key, so it is copied onto
	// every job here, once.
	Verbose bool
	// Version is what a generated header records; nil means no header is
	// written and there is none to record.
	Version func() (string, bool)
	// Fallback is the configuration the command's own flags and defaults give
	// dir.
	Fallback func(dir string) (generate.RoutesFileConfiguration, error)
	// KeepsHubCurrent makes a flagless run in a listed package's own directory
	// also select every other entry, to be read and not written, when the file
	// has a route_paths key, so the hub file can be built from all of them.
	KeepsHubCurrent bool
}

// onePackageWording is how a command that acts on one package says so, and how
// a person picks the package: Task follows the command name ("tests one
// package") and Where leads the directories ("pass one of").
type onePackageWording struct {
	Task, Where string
}

// packageRun is one package directory and the sets a command runs there: one
// per muxt.json entry under Key, in the file's order, or one for a run that
// flags configured. Key is the packages key the configuration came from, and
// File the absolute path of the muxt.json holding it; both are empty when
// flags gave the configuration. Given is the settings that did, by the
// spelling typed, so a report can name them.
type packageRun struct {
	File  string
	Key   string
	Dir   string
	Given givenSettings
	// Keyed is set on every package of a run that read every entry of the
	// muxt.json in its own directory, however many entries it holds, and on a
	// package whose key holds several sets in a run in one package directory.
	// Such a run names each set by Name; a run over one set of one package, or
	// one that flags configured, does not. Choosing by the package count in the
	// muxt.json's directory would change the output shape of a run when an
	// entry is added.
	Keyed bool
	// RoutePaths is the hub the muxt.json's route_paths key names, on every
	// package of a keyed run; nil without the key and in every other run.
	RoutePaths *routePathsTarget
	// ReadOnly is set on the packages of a run in one package's directory that
	// are not that package: they are loaded and resolved for the hub file, and
	// their directories are not written.
	ReadOnly bool
	Sets     []routeSet
}

// Named is whether a failure of the package says which package it is, since
// the run has more than one.
func (p packageRun) Named() bool { return p.Keyed || p.ReadOnly }

// routeSet is one muxt generate run: one configuration, which writes one
// routes file.
type routeSet struct {
	Config generate.RoutesFileConfiguration
}

// setRun is one set of a package, for a command that visits per set. It is a
// view of the package and set it names and holds no state of its own beyond
// the set's position, Index, the zero-based position of the set under the
// package's Key.
type setRun struct {
	*packageRun
	routeSet
	Index int
}

// setsOf is one setRun per set of every run, in run then set order. The
// setRuns point into runs, so a caller keeps runs unchanged while it uses them.
func setsOf(runs []packageRun) []setRun {
	var sets []setRun
	for i := range runs {
		for j, set := range runs[i].Sets {
			sets = append(sets, setRun{packageRun: &runs[i], routeSet: set, Index: j})
		}
	}
	return sets
}

// Name is what a run's output calls the set: its key, followed by its output
// file in parentheses when the key holds several entries, since the key alone
// would not tell them apart. Choosing by the entry count keeps a one-entry
// module's output as it was.
func (s setRun) Name() string {
	if len(s.Sets) > 1 {
		return fmt.Sprintf("%s (%s)", s.Key, s.Config.OutputFileName)
	}
	return s.Key
}

// Where is the set's entry as a muxt.json spells its position, for an error to
// name. It is meaningless for a set the command line configured.
func (s setRun) Where() string { return composite.SetWhere(s.Key, s.Index) }

// routePathsTarget is the package that holds route_paths.go and where the file
// goes. Hub has no Namespace: each package's own is made from its import path.
type routePathsTarget struct {
	Hub generate.Hub
	// Dir is the hub package's directory, and OutputFile the base name of the
	// file written there.
	Dir        string
	OutputFile string
}

// newRoutePathsTarget reads the hub of f, the package in its own directory.
func newRoutePathsTarget(f *composite.File) (*routePathsTarget, error) {
	importPath, err := f.KeyFor(f.Dir)
	if err != nil {
		return nil, fmt.Errorf("muxt.json entry for %s: %w", f.Dir, err)
	}
	outputFile := cmp.Or(f.RoutePaths.OutputFile, generate.DefaultRoutePathsFileName)
	name, err := load.HubPackageName(f.Dir, importPath, outputFile)
	if err != nil {
		return nil, err
	}
	names := generate.HubTypeNames{
		Paths:                     f.RoutePaths.OutputPathsType,
		RouteBuilder:              f.RoutePaths.OutputRouteBuilderType,
		Route:                     f.RoutePaths.OutputRouteType,
		PackagePathsSuffix:        f.RoutePaths.OutputPackagePathsTypeSuffix,
		PackageRouteBuilderSuffix: f.RoutePaths.OutputPackageRouteBuilderTypeSuffix,
	}
	return &routePathsTarget{Hub: generate.Hub{ImportPath: importPath, PackageName: name, Names: names}, Dir: f.Dir, OutputFile: outputFile}, nil
}

// selection is what selectJobs chose.
type selection struct {
	// File is the muxt.json read, nil when there is none.
	File *composite.File
	// Packages is never empty on success, and is in key order.
	Packages []packageRun
}

// Jobs is one setRun per set of every package, in package then set order,
// for a command that still reads a flat list.
func (s selection) Jobs() []setRun { return setsOf(s.Packages) }

// EntryFile is the muxt.json job's configuration came from, nil when the
// command's flags and defaults configured it, so its report names no file.
func (s selection) EntryFile(job setRun) *composite.File {
	if job.File == "" {
		return nil
	}
	return s.File
}

// selectJobs chooses the package directories and configurations a command
// acts on in dir, from the nearest muxt.json and the setting flags given.
// Flags win outright so a command line never half-merges with a file: a
// setting flag in a directory with an entry is an error, and in any other
// directory the Fallback decides. It validates every job's directory and entry
// before returning any. flags may be nil when nothing was given.
func selectJobs(spec jobSpec, dir string, flags *pflag.FlagSet, getEnv func(string) string) (selection, error) {
	var given givenSettings
	if flags != nil {
		given = givenFlags(flags, spec.Settings)
	}
	// A path that is not a directory is read from no muxt.json: the runner
	// says what is wrong with it, which no entry's message can.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fallbackSelection(spec, dir, nil, given)
	}
	lookup, err := lookupMuxtJSONEntry(dir)
	if err != nil {
		return selection{}, err
	}
	f := lookup.File
	if len(given) > 0 {
		if lookup.HasEntry {
			return selection{}, given.configuredDirectory(f.Name, spec.Name)
		}
		return fallbackSelection(spec, dir, f, given)
	}
	if f == nil {
		return fallbackSelection(spec, dir, nil, nil)
	}
	ownKey, err := f.KeyFor(f.Dir)
	if err != nil {
		return selection{}, fmt.Errorf("muxt.json entry for %s: %w", f.Dir, err)
	}
	keys := []string{lookup.Key}
	keyed := false
	hubFromChild := false
	if lookup.Key != ownKey {
		if !lookup.HasEntry {
			if spec.RequireEntry {
				return selection{}, &composite.MissingEntryError{Path: f.Name, Key: lookup.Key}
			}
			return fallbackSelection(spec, dir, f, nil)
		}
		if spec.KeepsHubCurrent && f.RoutePaths != nil {
			keys = f.Keys()
			hubFromChild = true
		}
	} else {
		if len(f.Packages) == 0 {
			if spec.RequireEntry {
				return selection{}, &composite.EmptyPackagesError{Path: f.Name}
			}
			return fallbackSelection(spec, dir, f, nil)
		}
		keys = f.Keys()
		if spec.Scope == itsEntry || spec.Scope == itsEntryOnly {
			// Only the directory's own entry is read, so another entry's
			// mistake cannot fail a command that acts on one package.
			if !lookup.HasEntry {
				if spec.Scope == itsEntryOnly {
					return selection{}, severalPackages(spec, f)
				}
				return fallbackSelection(spec, dir, f, nil)
			}
			keys = []string{lookup.Key}
		} else {
			keyed = true
		}
	}
	version := spec.Version
	if version == nil {
		version = noVersion
	}
	var routePaths *routePathsTarget
	if (keyed || hubFromChild) && f.RoutePaths != nil {
		if routePaths, err = newRoutePathsTarget(f); err != nil {
			return selection{}, err
		}
	}
	var runs []packageRun
	keyOf := map[string]string{} // a real directory to the first key that reached it
	for _, key := range keys {
		entryDir := f.DirOf(key)
		realDir, err := requireEntryDirectory(f, key, entryDir)
		if err != nil {
			return selection{}, err
		}
		if other, repeated := keyOf[realDir]; repeated {
			return selection{}, &composite.SameDirectoryError{Path: f.Name, Key: key, Other: other, Dir: realDir}
		}
		keyOf[realDir] = key
		sets, _ := f.Resolve(key)
		if f.RoutePaths != nil && len(sets) > 1 {
			return selection{}, &composite.RoutePathsSetsError{Path: f.Name, Key: key, Sets: len(sets)}
		}
		members := distinctSetMembers()
		firstWith := make([]map[string]int, len(members)) // per member, a value to the first entry that has it
		for m := range firstWith {
			firstWith[m] = map[string]int{}
		}
		// One key with several sets: nothing but a name tells them apart.
		setsNeedNames := !hubFromChild && len(sets) > 1
		run := packageRun{File: f.Name, Key: key, Dir: entryDir, Keyed: keyed || setsNeedNames, RoutePaths: routePaths, ReadOnly: hubFromChild && key != lookup.Key}
		for i, set := range sets {
			config, err := entryConfiguration(set, fromMuxtJSON, generateEnv{Dir: entryDir, GetEnv: getEnv, Version: version})
			if err != nil {
				return selection{}, &composite.InvalidEntryError{Path: f.Name, Where: composite.SetWhere(key, i), Err: err}
			}
			for m, member := range members {
				value := member.value(config)
				if first, clash := firstWith[m][value]; clash {
					return selection{}, &composite.SetClashError{Path: f.Name, Key: key, Member: member.name, Value: value, First: first, Second: i}
				}
				firstWith[m][value] = i
			}
			config.Verbose = spec.Verbose
			run.Sets = append(run.Sets, routeSet{Config: config})
		}
		runs = append(runs, run)
	}
	return selection{File: f, Packages: runs}, nil
}

// setMember is a muxt.json member whose resolved value must differ between the
// entries of one key, and how to read that value from an entry's configuration.
type setMember struct {
	name  string
	value func(generate.RoutesFileConfiguration) string
}

// distinctSetMembers are the members two entries of one key may not share, in
// the order a clash is reported. The files are written by one generate run
// each and owned by routes function, and the declared types would be declared
// twice in one package, which does not compile. They are compared after
// defaults, so two entries that leave one unset clash.
func distinctSetMembers() []setMember {
	return []setMember{
		// Cleaned, so "routes.go" and "./routes.go" are the one file they are.
		{"output_file", func(c generate.RoutesFileConfiguration) string { return filepath.Clean(c.OutputFileName) }},
		{"output_routes_func", func(c generate.RoutesFileConfiguration) string { return c.RoutesFunction }},
		{"output_receiver_interface", func(c generate.RoutesFileConfiguration) string { return c.ReceiverInterface }},
		{"output_template_data_type", func(c generate.RoutesFileConfiguration) string { return c.TemplateDataType }},
		{"output_template_route_paths_type", func(c generate.RoutesFileConfiguration) string { return c.TemplateRoutePathsTypeName }},
	}
}

// onePerDirectory folds the jobs of each directory into the first, in the
// order the directories first appear. Several sets of one key are one package,
// and a command that runs go test acts on a package, not on a route set:
// running it once per set would repeat every test. The folded job stands for
// the package, so a caller names it by Key, not Name, which would single out
// the first set's file. It carries the first set's configuration except for
// its templates variables, which are the union of every set's, so what a run
// attributes to the package covers all of its sets' template files. No
// caller's slice is changed.
func onePerDirectory(jobs []setRun) []setRun {
	var dirs []string // in order of first appearance
	byDir := map[string][]setRun{}
	for _, job := range jobs {
		if _, seen := byDir[job.Dir]; !seen {
			dirs = append(dirs, job.Dir)
		}
		byDir[job.Dir] = append(byDir[job.Dir], job)
	}
	folded := make([]setRun, 0, len(dirs))
	for _, dir := range dirs {
		sets := byDir[dir]
		job := sets[0]
		job.Config.TemplatesVariables = templatesVariables(sets)
		folded = append(folded, job)
	}
	return folded
}

// templatesVariables is the templates variables of sets, the sets of one
// directory, each once, in the order they are first named. One set answers its
// own slice as it is, so a package with one set reads nil or empty as its
// configuration does; several build a new slice, nil when they name none.
func templatesVariables(sets []setRun) []string {
	if len(sets) == 1 {
		return sets[0].Config.TemplatesVariables
	}
	var union []string
	for _, set := range sets {
		for _, name := range set.Config.TemplatesVariables {
			if !slices.Contains(union, name) {
				union = append(union, name)
			}
		}
	}
	return union
}

// severalPackages names the directories of f's entries, relative to f, for a
// command that tests one package.
func severalPackages(spec jobSpec, f *composite.File) error {
	var dirs []string
	for _, key := range f.Keys() {
		rel, err := filepath.Rel(f.Dir, f.DirOf(key))
		if err != nil {
			return fmt.Errorf("muxt.json entry %s: %w", key, err)
		}
		dirs = append(dirs, filepath.ToSlash(rel))
	}
	return &composite.SeveralPackagesError{Path: f.Name, Command: spec.Name, Task: spec.OnePackage.Task, Where: spec.OnePackage.Where, Dirs: dirs}
}

// fallbackSelection is the one job the command's own flags and defaults give
// dir. given is the setting flags that did.
func fallbackSelection(spec jobSpec, dir string, f *composite.File, given givenSettings) (selection, error) {
	config, err := spec.Fallback(dir)
	if err != nil {
		return selection{}, err
	}
	config.Verbose = spec.Verbose
	run := packageRun{Dir: dir, Given: given, Sets: []routeSet{{Config: config}}}
	return selection{File: f, Packages: []packageRun{run}}, nil
}

// entryVariableError returns the error naming job's muxt.json entry when err
// says the package lacks the templates variable, so the fix is in the file.
// ok is false for any other err, and for a job the command line configured.
func entryVariableError(job setRun, err error) (entryErr error, ok bool) {
	missing, ok := errors.AsType[*check.VariableNotFoundError](err)
	if !ok || job.File == "" {
		return nil, false
	}
	return &composite.NoVariableError{Path: job.File, Key: job.Key, Set: job.Index, Package: missing.Package, Variable: missing.Variable}, true
}

// entryHook is analysis.NewModule's answer to what the muxt.json holds for a
// package directory. It asks selectJobs, as every other command does, so an
// entry that breaks a rule fails here too instead of passing as configured.
func entryHook(getEnv func(string) string) func(dir string) (analysis.Entry, error) {
	spec := jobSpec{
		Name:  exploreModuleCommandName,
		Scope: itsEntry,
		Fallback: func(dir string) (generate.RoutesFileConfiguration, error) {
			return defaultConfiguration(dir, getEnv)
		},
	}
	return func(dir string) (analysis.Entry, error) {
		sel, err := selectJobs(spec, dir, nil, getEnv)
		if err != nil {
			return analysis.Entry{}, err
		}
		if sel.Jobs()[0].File == "" {
			return analysis.Entry{}, nil
		}
		entry := analysis.Entry{Configured: true}
		if sel.File.RoutePaths != nil {
			target, err := newRoutePathsTarget(sel.File)
			if err != nil {
				return analysis.Entry{}, err
			}
			entry.RoutePathsType = target.Hub.PackageName + "." + target.Hub.Names.PathsType()
		}
		return entry, nil
	}
}
