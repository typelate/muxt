// Package composite is the muxt.json file, which composes the generate
// configuration of several packages in one module so a flagless muxt generate
// knows what to write for each.
package composite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
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
	Defaults Entry            `json:"package_defaults,omitempty"`
	Packages map[string]Entry `json:"packages"`
}

// Resolve returns the entry for key with Defaults applied. A key the entry
// leaves out takes the default; a key the entry holds, even set to false or an
// empty list, is kept whole. found is false when f has no entry under key.
func (f *File) Resolve(key string) (e Entry, found bool) {
	own, found := f.Packages[key]
	if !found {
		return Entry{}, false
	}
	e = Entry{}
	maps.Copy(e, f.Defaults)
	maps.Copy(e, own)
	return e, true
}

// Find reads the first muxt.json found walking up from wd, starting at wd
// itself. The walk stops at the module root: the first directory holding a
// go.mod is checked for muxt.json and no directory above it is. A muxt.json
// above the module belongs to another project, so it is not read. When no
// go.mod is found on the way up, the walk goes to the filesystem root. found is
// false when the walk ends without a muxt.json. keys says which keys an entry
// may hold and which are deprecated.
func Find(wd string, keys Keys) (f *File, found bool, err error) {
	start, err := filepath.Abs(wd)
	if err != nil {
		return nil, false, fmt.Errorf("absolute path of %s: %w", wd, err)
	}
	dir := start
	for {
		f, err := read(filepath.Join(dir, FileName), keys)
		if err == nil {
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
// fails with an *AlreadyConfiguredError when one configures dir, since a nearer
// file would shadow it, and with a *NoModuleError when no go.mod is at or above
// dir, since no command could read the file. keys is as Find takes it.
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
		return &NoModuleError{Path: path}
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
	if f.Packages == nil {
		f.Packages = map[string]Entry{}
	}
	// An entry that is nil would be written as null, which reads back as {}.
	for key, e := range f.Packages {
		if e == nil {
			f.Packages[key] = Entry{}
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "\t")
	if err := enc.Encode(f); err != nil {
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
	// Syntax first: decoding into a RawMessage checks nothing else.
	var whole json.RawMessage
	if err := json.Unmarshal(data, &whole); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			line, col := position(data, syntax.Offset)
			return nil, &ParseError{Path: path, Line: line, Col: col, Err: syntax}
		}
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	// A wrong outer shape fails the raw read, and the typed read below says
	// so in the json package's words.
	if raw, err := readRaw(data); err == nil {
		if err := checkKinds(path, raw, keys); err != nil {
			return nil, err
		}
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	dropNulls(f.Defaults)
	for key, e := range f.Packages {
		if e == nil {
			e = Entry{}
			f.Packages[key] = e
		}
		dropNulls(e)
	}
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
	if err := f.checkKeys(data, keys); err != nil {
		return nil, err
	}
	return &f, nil
}

// checkKeys returns the checkPackageKey error for the first packages key, in
// sorted order, that is not an import path of a package at or below the file's
// directory, or the entry-key errors of checkEntryKeys for package_defaults
// and then for each packages entry. json.Unmarshal drops unknown keys without a
// word, so the keys are read again here.
func (f *File) checkKeys(data []byte, keys Keys) error {
	var raw struct {
		PackageDefaults map[string]json.RawMessage            `json:"package_defaults"`
		Packages        map[string]map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing %s: %w", f.Name, err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("parsing %s: %w", f.Name, err)
	}
	topKnown := []string{"package_defaults", "packages"}
	for _, key := range slices.Sorted(maps.Keys(top)) {
		if !slices.Contains(topKnown, key) {
			return &UnknownKeyError{Path: f.Name, Key: key, Suggestion: closestKey(key, topKnown)}
		}
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
		if err := checkEntryKeys(f.Name, fmt.Sprintf("packages[%q]", key), raw.Packages[key], keys.Deprecated, known); err != nil {
			return err
		}
	}
	return nil
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
	PackageDefaults map[string]json.RawMessage            `json:"package_defaults"`
	Packages        map[string]map[string]json.RawMessage `json:"packages"`
}

// readRaw decodes data into the entries as written. It fails when the shape
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
		if err := checkEntryKinds(path, fmt.Sprintf("packages[%q]", key), raw.Packages[key], keys); err != nil {
			return err
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
