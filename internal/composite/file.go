// Package composite is the muxt.json file, which composes the generate
// configuration of several packages in one module so a flagless muxt generate
// knows what to write for each.
package composite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// FileName is the name of the configuration file muxt looks for.
const FileName = "muxt.json"

// File is a muxt.json. Dir is the directory holding the file, set by Find.
// Packages is keyed by the full import path of each package, as Go names it,
// such as "example.com/internal/hypertext/workspace".
type File struct {
	// Dir is the absolute directory holding the file, so package directories
	// can be found from it.
	Dir string `json:"-"`
	// module is the module path of the nearest go.mod at or above Dir, and
	// moduleDir the directory holding it. An import path in the module is
	// the module path joined with a directory below moduleDir.
	module, moduleDir string
	// Name is the file's absolute path, the string an editor can open.
	// Errors name the file this way.
	Name string `json:"-"`
	// Defaults holds the keys every entry takes unless the entry sets them
	// itself. It is a separate object so one shared value is written once.
	Defaults Entry `json:"package_defaults,omitempty"`
	// Packages holds the entries of each key, one per muxt generate run.
	Packages map[string][]Entry `json:"packages"`
	// RoutePaths is the route_paths object. nil means the key is absent.
	RoutePaths *RoutePaths `json:"route_paths,omitempty"`
}

// RoutePaths is the route_paths object of a muxt.json.
type RoutePaths struct {
	OutputFile string `json:"output_file,omitempty"`
	// OutputPathsType names the hub's paths type. Its default is TemplateRoutePaths.
	OutputPathsType string `json:"output_paths_type,omitempty"`
	// OutputRouteBuilderType names the hub's route builder type. Its default is TemplateRouteBuilder.
	OutputRouteBuilderType string `json:"output_route_builder_type,omitempty"`
	// OutputRouteType names the hub's route type. Its default is TemplateRoute.
	OutputRouteType string `json:"output_route_type,omitempty"`
	// OutputPackagePathsTypeSuffix is the suffix a package's paths type takes after its namespace. Its default is TemplateRoutePaths.
	OutputPackagePathsTypeSuffix string `json:"output_package_paths_type_suffix,omitempty"`
	// OutputPackageRouteBuilderTypeSuffix is the suffix a package's route builder type takes after its namespace. Its default is TemplateRouteBuilder.
	OutputPackageRouteBuilderTypeSuffix string `json:"output_package_route_builder_type_suffix,omitempty"`
}

// Resolve returns each entry under key, in the file's order, with Defaults
// applied to each. A key an entry leaves out takes the default; a key it holds,
// even set to false or an empty list, is kept whole. found is false when f has
// no entry under key.
func (f *File) Resolve(key string) (sets []Entry, found bool) {
	own, found := f.Packages[key]
	if !found {
		return nil, false
	}
	sets = make([]Entry, len(own))
	for i, entry := range own {
		e := Entry{}
		maps.Copy(e, f.Defaults)
		maps.Copy(e, entry)
		sets[i] = e
	}
	return sets, true
}

// Find reads the first muxt.json found walking up from wd, starting at wd
// itself. The walk stops at the module root: the first directory holding a
// go.mod is checked for muxt.json and no directory above it is. A muxt.json
// above the module belongs to another project, so it is not read. When no
// go.mod is found on the way up, the walk goes to the filesystem root. found is
// false when the walk ends without a muxt.json. Once a file is found the walk
// goes on to the module root, and a second file there is a *NestedError, since
// two files on one path would configure a package two ways. keys says which
// keys an entry may hold and which are deprecated.
func Find(wd string, keys Keys) (f *File, found bool, err error) {
	start, err := filepath.Abs(wd)
	if err != nil {
		return nil, false, fmt.Errorf("absolute path of %s: %w", wd, err)
	}
	dir := start
	for {
		f, err := read(filepath.Join(dir, FileName), keys)
		if err == nil {
			if err := f.requireNoneAbove(); err != nil {
				return nil, false, err
			}
			return f, true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, false, err
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return nil, false, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, false, nil
		}
		dir = parent
	}
}

// Create writes an empty muxt.json into dir, the file a flagless generate
// reads before any entry is added. It looks for a file the way Find does and
// fails with an *AlreadyConfiguredError when one configures dir, with a
// *NestedError when a muxt.json is below dir in the same module, and with a
// *NoModuleError when no go.mod is at or above dir, since no command could read
// the file. keys is as Find takes it.
func Create(dir string, keys Keys) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("absolute path of %s: %w", dir, err)
	}
	existing, found, err := Find(abs, keys)
	if err != nil {
		return err
	}
	if found {
		return &AlreadyConfiguredError{Path: existing.Name}
	}
	path := filepath.Join(abs, FileName)
	_, _, hasModule, err := moduleOf(abs)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if !hasModule {
		return &NoModuleError{Path: path, Creating: true}
	}
	if err := requireNoneBelow(abs, path); err != nil {
		return err
	}
	return writeNew(path, []byte("{\n\t\"packages\": {}\n}\n"))
}

// writeNew creates path with data and fails with an *ExistsError, writing
// nothing, when path already exists.
func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return &ExistsError{Path: path}
	}
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// Save rewrites the file in its one stable form: tab indent, package_defaults
// before packages, keys sorted, one trailing newline. It writes a temporary
// file beside it and renames that over it, so a failure leaves the old file.
func (f *File) Save() error {
	// Encode a copy: an entry that is nil would be written as null, which
	// reads back as {}, and the caller's file is not ours to change.
	out := savedFile{Defaults: f.Defaults, RoutePaths: f.RoutePaths}
	out.Packages = make(map[string]savedSets, len(f.Packages))
	for key, sets := range f.Packages {
		cloned := make(savedSets, len(sets))
		for i, e := range sets {
			if e == nil {
				e = Entry{}
			}
			cloned[i] = e
		}
		out.Packages[key] = cloned
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "\t")
	if err := enc.Encode(&out); err != nil {
		return fmt.Errorf("encoding %s: %w", f.Name, err)
	}
	tmp, err := os.CreateTemp(f.Dir, FileName+".*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", f.Name, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", f.Name, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", f.Name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", f.Name, err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(f.Dir, FileName)); err != nil {
		return fmt.Errorf("writing %s: %w", f.Name, err)
	}
	return nil
}

// savedFile is a File as Save encodes it.
type savedFile struct {
	Defaults   Entry                `json:"package_defaults,omitempty"`
	Packages   map[string]savedSets `json:"packages"`
	RoutePaths *RoutePaths          `json:"route_paths,omitempty"`
}

// savedSets is the transitional counterpart of entrySets: a key with one entry
// is written as the bare object, the form every reader of an older muxt.json
// knows. Delete it, and write every key as a list, with the list form.
type savedSets []Entry

// MarshalJSON writes one entry as an object and any other count as a list.
func (s savedSets) MarshalJSON() ([]byte, error) {
	// The markup characters stay as written, as Save's own encoder leaves them.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	var err error
	if len(s) == 1 {
		err = enc.Encode(s[0])
	} else {
		err = enc.Encode([]Entry(s))
	}
	if err != nil {
		return nil, fmt.Errorf("encoding entries: %w", err)
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

// isNull reports whether value is the JSON null.
func isNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

// dropNulls removes each key whose value is null, so a key that is present is
// a setting given.
func dropNulls(e Entry) {
	maps.DeleteFunc(e, func(_ string, value json.RawMessage) bool { return isNull(value) })
}

// KeyFor returns the import path of dir, the key its Packages entry uses: the
// module path of the nearest go.mod joined with dir's slash path from the
// directory holding that go.mod. Go names a package this way, so the key
// reads the same wherever the file sits in the module. dir must be the file's
// directory or below it.
func (f *File) KeyFor(dir string) (string, error) {
	rel, err := filepath.Rel(f.moduleDir, dir)
	if err != nil {
		return "", fmt.Errorf("key for %s: %w", dir, err)
	}
	return path.Join(f.module, filepath.ToSlash(rel)), nil
}

// Keys returns the packages keys sorted. Find rejects a key that is not an
// import path in clean form, so the keys are the ones a person wrote.
func (f *File) Keys() []string {
	return slices.Sorted(maps.Keys(f.Packages))
}

// Add puts entry under key in f's packages. It fails with an *ExistsError
// naming the key when f already holds it, so of two runs adding one key under
// Update's lock only the first writes.
func (f *File) Add(key string, entry Entry) error {
	if _, exists := f.Packages[key]; exists {
		return &ExistsError{Path: f.Name, Where: SetWhere(key, 0)}
	}
	if f.Packages == nil {
		f.Packages = map[string][]Entry{}
	}
	f.Packages[key] = []Entry{entry}
	return nil
}

// SetWhere returns the position in a muxt.json of the i-th entry, counting from
// zero, under key: packages["example.com/m"][0]. A position is always written
// with its index, also for a key with one entry, so an error reads the same
// when a second entry is added.
func SetWhere(key string, i int) string {
	return fmt.Sprintf("packages[%q][%d]", key, i)
}

// DirOf returns the absolute directory of a packages key, which Find has
// checked is an import path in f's module.
func (f *File) DirOf(key string) string {
	rel := strings.TrimPrefix(strings.TrimPrefix(key, f.module), "/")
	return filepath.Join(f.moduleDir, filepath.FromSlash(rel))
}

// Entry is one muxt.json object as the file holds it: each key a setting
// given, its value as written. Keys are the flag names without "--", with
// each "-" written as "_" (see Key); checkKeys rejects the dashed form so a
// file has one spelling per key. A key that is not there leaves the setting
// to its default, and read drops null values, so null means the same. composite
// does not know what a value means: the command that owns the settings
// decodes it.
type Entry map[string]json.RawMessage

// read reads the muxt.json at path, an absolute path its errors name the file
// by. A missing file is an error that matches fs.ErrNotExist.
func read(path string, keys Keys) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	// The file is decoded once, into the raw shape. The json package checks
	// the syntax of the whole input before it decodes any of it, so a syntax
	// error is reported before a value of the wrong kind.
	raw, err := readRaw(data)
	if err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			line, col := position(data, syntax.Offset)
			return nil, &ParseError{Path: path, Line: line, Col: col, Err: syntax}
		}
	}
	// A wrong outer shape, such as a list where packages goes, is reported
	// from the tokens: the json package would name this package's Go types.
	if shapeErr := checkShape(path, data); shapeErr != nil {
		return nil, shapeErr
	}
	if err != nil {
		// The json package matches a struct field in any letter case, so a
		// "Packages" holding a list fails as a Go type: name it as a key the
		// file does not know instead.
		if kindErr := checkRoutePathsKinds(path, data); kindErr != nil {
			return nil, kindErr
		}
		if keyErr := checkTopKeys(path, data); keyErr != nil {
			return nil, keyErr
		}
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := checkKinds(path, raw, keys); err != nil {
		return nil, err
	}
	f := File{Defaults: raw.PackageDefaults, Packages: raw.entries(), RoutePaths: raw.RoutePaths}
	f.Dir = filepath.Dir(path)
	f.Name = path
	var hasModule bool
	f.module, f.moduleDir, hasModule, err = moduleOf(f.Dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if !hasModule {
		return nil, &NoModuleError{Path: path}
	}
	// Keys are checked before nulls are dropped, since f shares raw's maps
	// and an unknown key set to null is still a key the file holds.
	if err := f.checkKeys(data, raw, keys); err != nil {
		return nil, err
	}
	dropNulls(f.Defaults)
	for _, sets := range f.Packages {
		for i, e := range sets {
			if e == nil {
				e = Entry{}
				sets[i] = e
			}
			dropNulls(e)
		}
	}
	return &f, nil
}

// checkKeys returns the checkPackageKey error for the first packages key, in
// sorted order, that is not an import path of a package at or below the file's
// directory, or the entry-key errors of checkEntryKeys for package_defaults
// and then for each packages entry. json.Unmarshal drops unknown keys without a
// word, so the top-level keys are read again from data here.
func (f *File) checkKeys(data []byte, raw rawFile, keys Keys) error {
	if err := checkTopKeys(f.Name, data); err != nil {
		return err
	}
	if err := checkRoutePathsKeys(f.Name, data); err != nil {
		return err
	}
	if err := checkRoutePathsValues(f.Name, raw.RoutePaths); err != nil {
		return err
	}
	// Sorted, so a did-you-mean hint between two equally close keys is the
	// same every run.
	known := slices.Sorted(maps.Keys(keys.Kinds))
	if err := checkEntryKeys(f.Name, "package_defaults", raw.PackageDefaults, keys.Deprecated, known); err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(raw.Packages)) {
		if err := f.checkPackageKey(key); err != nil {
			return err
		}
		for i, entry := range raw.Packages[key] {
			if err := checkEntryKeys(f.Name, SetWhere(key, i), entry, keys.Deprecated, known); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkTopKeys returns an *UnknownKeyError for the first top-level key of
// data, in sorted order, other than package_defaults and packages, matched in
// their exact letter case. data is a JSON object, the file at path.
func checkTopKeys(path string, data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	topKnown := []string{"package_defaults", "packages", "route_paths"}
	for _, key := range slices.Sorted(maps.Keys(top)) {
		if !slices.Contains(topKnown, key) {
			return &UnknownKeyError{Path: path, Key: key, Suggestion: closestKey(key, topKnown)}
		}
	}
	return nil
}

// checkRoutePathsKeys returns an *UnknownKeyError for the first member of the
// route_paths object in data, in sorted order, that RoutePaths does not read.
// json.Unmarshal drops such a member without a word, so the members are read
// again from data here. A route_paths that is absent or null holds none.
func checkRoutePathsKeys(path string, data []byte) error {
	var top struct {
		RoutePaths map[string]json.RawMessage `json:"route_paths"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	// Each name is the JSON name of a RoutePaths field. Add a name here when a
	// field is added, or a muxt.json holding it is rejected.
	routePathsKnown := []string{"output_file", "output_paths_type", "output_route_builder_type", "output_route_type", "output_package_paths_type_suffix", "output_package_route_builder_type_suffix"}
	for _, key := range slices.Sorted(maps.Keys(top.RoutePaths)) {
		if !slices.Contains(routePathsKnown, key) {
			return &UnknownKeyError{Path: path, Where: "route_paths", Key: key, Suggestion: closestKey(key, routePathsKnown)}
		}
	}
	return nil
}

// checkRoutePathsKinds returns a *ValueTypeError for an output_file in data that
// is not a JSON string. The json package fails on such a value with a Go type
// name, so the members are read again from data here, the way checkKinds reads
// an entry's.
func checkRoutePathsKinds(path string, data []byte) error {
	var top struct {
		RoutePaths map[string]json.RawMessage `json:"route_paths"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	kinds := Keys{Kinds: map[string]Kind{
		"output_file":                              KindString,
		"output_paths_type":                        KindString,
		"output_route_builder_type":                KindString,
		"output_route_type":                        KindString,
		"output_package_paths_type_suffix":         KindString,
		"output_package_route_builder_type_suffix": KindString,
	}}
	return checkEntryKinds(path, "route_paths", top.RoutePaths, kinds)
}

// checkRoutePathsValues returns a *RouteValueError for the first member of
// route_paths whose value cannot be the name it configures, in the order the
// RoutePaths fields are declared. An output_file must be a Go file name: the
// generator joins it onto the muxt.json directory, so a name with a directory
// part would write outside it. The three type names must be exported Go
// identifiers, since other packages name them. The two suffixes follow a
// package's namespace, so they hold identifier characters only. An absent or
// empty member is fine: the default is used.
func checkRoutePathsValues(path string, rp *RoutePaths) error {
	if rp == nil {
		return nil
	}
	exported := func(s string) bool { return token.IsIdentifier(s) && token.IsExported(s) }
	suffix := func(s string) bool { return token.IsIdentifier("X" + s) }
	checks := []struct {
		key, value, rule string
		valid            func(string) bool
	}{
		{"output_file", rp.OutputFile, "must be a Go file name in this directory", isGoFileName},
		{"output_paths_type", rp.OutputPathsType, "must be an exported Go identifier", exported},
		{"output_route_builder_type", rp.OutputRouteBuilderType, "must be an exported Go identifier", exported},
		{"output_route_type", rp.OutputRouteType, "must be an exported Go identifier", exported},
		{"output_package_paths_type_suffix", rp.OutputPackagePathsTypeSuffix, "must be letters, digits and underscores", suffix},
		{"output_package_route_builder_type_suffix", rp.OutputPackageRouteBuilderTypeSuffix, "must be letters, digits and underscores", suffix},
	}
	for _, c := range checks {
		if c.value != "" && !c.valid(c.value) {
			return &RouteValueError{Path: path, Where: "route_paths", Key: c.key, Value: c.value, Rule: c.rule}
		}
	}
	return nil
}

// isGoFileName reports whether name is a plain file name ending in ".go", with
// something before the extension and no separator or whitespace.
func isGoFileName(name string) bool {
	base := strings.TrimSuffix(name, ".go")
	return base != "" && base != name && !strings.ContainsAny(name, `/\`) && !strings.ContainsFunc(name, unicode.IsSpace)
}

// checkPackageKey returns a *KeyFormError for a key that is a directory
// relative to the file, or an import path of the module that is not clean, and
// an *OtherModuleError for one outside the module, a *KeyOutsideError for one
// outside the file's directory, so a checked-out muxt.json cannot point generate at, or write
// into, another place. Two spellings of one package would be two entries, so
// each key is the form path.Clean gives it.
func (f *File) checkPackageKey(key string) error {
	if clean := path.Clean(key); f.inModule(clean) {
		if clean != key {
			return &KeyFormError{Path: f.Name, Key: key, ImportPath: clean}
		}
		if rel, err := filepath.Rel(f.Dir, f.DirOf(key)); err != nil || !filepath.IsLocal(rel) {
			return &KeyOutsideError{Path: f.Name, Key: key}
		}
		onDisk, err := f.spelledOnDisk(key)
		if err != nil {
			return err
		}
		if onDisk != key {
			return &KeyCaseError{Path: f.Name, Key: key, OnDisk: onDisk}
		}
		return nil
	}
	// A key that is not an import path of the module is either a directory
	// the way keys used to be written, which has an import path to name, or
	// the import path of some other package.
	if !isRelative(key) {
		return &OtherModuleError{Path: f.Name, Key: key, Module: f.module}
	}
	dir := filepath.FromSlash(key)
	if !filepath.IsLocal(dir) {
		return &KeyOutsideError{Path: f.Name, Key: key}
	}
	want, err := f.KeyFor(filepath.Join(f.Dir, dir))
	if err != nil {
		return err
	}
	return &KeyFormError{Path: f.Name, Key: key, ImportPath: want}
}

// inModule reports whether importPath is the file's module path or below it.
func (f *File) inModule(importPath string) bool {
	return importPath == f.module || strings.HasPrefix(importPath, f.module+"/")
}

// isRelative reports whether key is written as a directory rather than an
// import path. The first element of an import path that is not in the file's
// own module is a domain, which holds a dot; "workspace" and "." do not.
func isRelative(key string) bool {
	first, _, _ := strings.Cut(key, "/")
	return !strings.Contains(first, ".") || first == "." || first == ".."
}

// checkEntryKeys returns the error for the first key, in sorted order, of the
// entry where names: a *DeprecatedKeyError, a *DashedKeyError, or an
// *UnknownKeyError. known is the sorted keys an entry accepts.
func checkEntryKeys(path, where string, keys map[string]json.RawMessage, deprecated map[string]string, known []string) error {
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if replacement, ok := deprecated[key]; ok {
			return &DeprecatedKeyError{Path: path, Where: where, Key: key, Replacement: replacement}
		}
		if slices.Contains(known, key) {
			continue
		}
		// The dashed spelling is named only when the underscore form is a
		// key, so a typo with dashes still reads as an unknown key.
		if underscored := strings.ReplaceAll(key, "-", "_"); slices.Contains(known, underscored) {
			return &DashedKeyError{Path: path, Where: where, Key: key, Underscored: underscored}
		}
		return &UnknownKeyError{Path: path, Where: where, Key: key, Suggestion: closestKey(key, known)}
	}
	return nil
}

// closestKey returns the key of known nearest to key by edit distance, or ""
// when none is within a fifth of key's length (at least one edit), so a hint
// is a likely typo and never a guess.
func closestKey(key string, known []string) string {
	best, bestDistance := "", max(1, len(key)/5)+1
	for _, k := range known {
		if d := editDistance(key, k); d < bestDistance {
			best, bestDistance = k, d
		}
	}
	return best
}

// editDistance returns the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// position returns the 1-based line and column of the byte before offset. The
// json package reports an offset one past the byte it rejected.
func position(data []byte, offset int64) (line, col int) {
	pos := max(int(offset)-1, 0)
	before := data[:pos]
	return 1 + bytes.Count(before, []byte{'\n'}), pos - bytes.LastIndexByte(before, '\n')
}

// spelledOnDisk returns key with each element below the module spelled as its
// directory is. A directory that does not exist is left as written: whether
// the package is there is for the go command to say.
func (f *File) spelledOnDisk(key string) (string, error) {
	rel := strings.TrimPrefix(strings.TrimPrefix(key, f.module), "/")
	if rel == "" {
		return key, nil
	}
	dir := f.moduleDir
	elements := strings.Split(rel, "/")
	for i, element := range elements {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return key, nil
			}
			return "", fmt.Errorf("reading %s: %w", dir, err)
		}
		found := ""
		for _, entry := range entries {
			if entry.Name() == element {
				found = element
				break
			}
			if found == "" && strings.EqualFold(entry.Name(), element) {
				found = entry.Name()
			}
		}
		if found == "" {
			return key, nil
		}
		elements[i] = found
		dir = filepath.Join(dir, found)
	}
	return path.Join(f.module, path.Join(elements...)), nil
}

// rawFile is the entries of a muxt.json as written, each value left as bytes,
// so a value's JSON type can be checked before any typed decoding names a Go
// struct field in its error.
type rawFile struct {
	PackageDefaults Entry                `json:"package_defaults"`
	Packages        map[string]entrySets `json:"packages"`
	RoutePaths      *RoutePaths          `json:"route_paths"`
}

// entries returns the entries under each packages key, without the type that
// reads them.
func (r rawFile) entries() map[string][]Entry {
	out := make(map[string][]Entry, len(r.Packages))
	for key, sets := range r.Packages {
		out[key] = []Entry(sets)
	}
	return out
}

// entrySets is the value under one packages key as it is read. It is the
// transitional type of a muxt.json written before a key held a list: a bare
// object is read as one entry. Delete it, and read a list, when the object
// form is refused.
type entrySets []Entry

// UnmarshalJSON reads a list of objects, and a bare object, or null, as one
// entry.
func (s *entrySets) UnmarshalJSON(data []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		var list []Entry
		if err := json.Unmarshal(data, &list); err != nil {
			return fmt.Errorf("reading the entries of a key: %w", err)
		}
		*s = list
		return nil
	}
	var one Entry
	if err := json.Unmarshal(data, &one); err != nil {
		return fmt.Errorf("reading an entry: %w", err)
	}
	*s = entrySets{one}
	return nil
}

// readRaw decodes data into the entries as written. It fails with a
// *json.SyntaxError when data is not JSON, and otherwise when the shape
// outside the values is wrong, such as a list where packages goes.
func readRaw(data []byte) (rawFile, error) {
	var raw rawFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return rawFile{}, fmt.Errorf("reading entries: %w", err)
	}
	return raw, nil
}

// checkKinds returns a *ValueTypeError for the first value, in the order
// package_defaults, then packages sorted, then each entry's keys sorted, that
// is not the JSON type its key takes. A key keys does not know is left to
// checkKeys, and null is an absent key, so neither is a kind error.
func checkKinds(path string, raw rawFile, keys Keys) error {
	if err := checkEntryKinds(path, "package_defaults", raw.PackageDefaults, keys); err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(raw.Packages)) {
		for i, entry := range raw.Packages[key] {
			if err := checkEntryKinds(path, SetWhere(key, i), entry, keys); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkEntryKinds is checkKinds for the entry where names.
func checkEntryKinds(path, where string, entry map[string]json.RawMessage, keys Keys) error {
	for _, key := range slices.Sorted(maps.Keys(entry)) {
		want, known := keys.Kinds[key]
		value := entry[key]
		if !known || isNull(value) {
			continue
		}
		var err error
		switch want {
		case KindBool:
			var v bool
			err = json.Unmarshal(value, &v)
		case KindList:
			var v []string
			err = json.Unmarshal(value, &v)
		default:
			var v string
			err = json.Unmarshal(value, &v)
		}
		if err != nil {
			return &ValueTypeError{Path: path, Where: where, Key: key, Want: want}
		}
	}
	return nil
}
