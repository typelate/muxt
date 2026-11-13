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
	"reflect"
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
	Defaults Entry            `json:"package_defaults,omitzero"`
	Packages map[string]Entry `json:"packages"`
}

// Resolve returns the entry for key with Defaults applied. A field the entry
// leaves unset takes the default; a field the entry sets, even to false or an
// empty list, is kept. found is false when f has no entry under key.
func (f *File) Resolve(key string) (e Entry, found bool) {
	e, found = f.Packages[key]
	if !found {
		return Entry{}, false
	}
	entry := reflect.ValueOf(&e).Elem()
	defaults := reflect.ValueOf(f.Defaults)
	for i := range entry.NumField() {
		if entry.Field(i).IsZero() {
			entry.Field(i).Set(defaults.Field(i))
		}
	}
	return e, true
}

// Find reads the first muxt.json found walking up from wd, starting at wd
// itself. The walk stops at the module root: the first directory holding a
// go.mod is checked for muxt.json and no directory above it is. A muxt.json
// above the module belongs to another project, so it is not read. When no
// go.mod is found on the way up, the walk goes to the filesystem root. found is
// false when the walk ends without a muxt.json. deprecated maps an old key to
// its replacement, as Find takes it.
func Find(wd string, deprecated map[string]string) (f *File, found bool, err error) {
	start, err := filepath.Abs(wd)
	if err != nil {
		return nil, false, fmt.Errorf("absolute path of %s: %w", wd, err)
	}
	dir := start
	for {
		f, err := read(filepath.Join(dir, FileName), deprecated)
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
// reads before any entry is added.
func Create(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("absolute path of %s: %w", dir, err)
	}
	return writeNew(filepath.Join(abs, FileName), []byte("{\n\t\"packages\": {}\n}\n"))
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

// Entry holds the generate flags of one package directory. JSON keys are the
// flag names without "--", with each "-" written as "_": underscores keep a
// key a single identifier-like word, and checkKeys rejects the dashed form so
// a file has one spelling per key. A nil field is absent from the file, so the
// flag's default applies. Fields follow the order `muxt generate --help` lists
// flags; verbose is not configuration and has no field.
type Entry struct {
	// InputStaticAssetsDirectory is --input-static-assets-directory.
	InputStaticAssetsDirectory *string `json:"input_static_assets_directory,omitempty"`
	// OutputDatastar is --output-datastar.
	OutputDatastar *bool `json:"output_datastar,omitempty"`
	// OutputExportedDefaultIdentifiers is --output-exported-default-identifiers.
	OutputExportedDefaultIdentifiers *bool `json:"output_exported_default_identifiers,omitempty"`
	// OutputFile is --output-file.
	OutputFile *string `json:"output_file,omitempty"`
	// OutputHTMX is --output-htmx.
	OutputHTMX *bool `json:"output_htmx,omitempty"`
	// OutputMultipartMaxMemory is --output-multipart-max-memory, in the form the flag accepts, such as "64MB".
	OutputMultipartMaxMemory *string `json:"output_multipart_max_memory,omitempty"`
	// OutputMultipleFiles is --output-multiple-files.
	OutputMultipleFiles *bool `json:"output_multiple_files,omitempty"`
	// OutputMuxtVersion is --output-muxt-version.
	OutputMuxtVersion *bool `json:"output_muxt_version,omitempty"`
	// OutputReceiverInterface is --output-receiver-interface.
	OutputReceiverInterface *string `json:"output_receiver_interface,omitempty"`
	// OutputRoutesFunc is --output-routes-func.
	OutputRoutesFunc *string `json:"output_routes_func,omitempty"`
	// OutputRoutesFuncWithLoggerParam is --output-routes-func-with-logger-param.
	OutputRoutesFuncWithLoggerParam *bool `json:"output_routes_func_with_logger_param,omitempty"`
	// OutputRoutesFuncWithMiddlewareParam is --output-routes-func-with-middleware-param.
	OutputRoutesFuncWithMiddlewareParam *bool `json:"output_routes_func_with_middleware_param,omitempty"`
	// OutputRoutesFuncWithPathPrefixParam is --output-routes-func-with-path-prefix-param.
	OutputRoutesFuncWithPathPrefixParam *bool `json:"output_routes_func_with_path_prefix_param,omitempty"`
	// OutputSSETemplateDataType is --output-sse-template-data-type.
	OutputSSETemplateDataType *string `json:"output_sse_template_data_type,omitempty"`
	// OutputStaticAssetsPath is --output-static-assets-path.
	OutputStaticAssetsPath *string `json:"output_static_assets_path,omitempty"`
	// OutputTemplateDataType is --output-template-data-type.
	OutputTemplateDataType *string `json:"output_template_data_type,omitempty"`
	// OutputTemplateRouteBuilderType is --output-template-route-builder-type.
	OutputTemplateRouteBuilderType *string `json:"output_template_route_builder_type,omitempty"`
	// OutputTemplateRoutePathsType is --output-template-route-paths-type.
	OutputTemplateRoutePathsType *string `json:"output_template_route_paths_type,omitempty"`
	// OutputTemplateRouteType is --output-template-route-type.
	OutputTemplateRouteType *string `json:"output_template_route_type,omitempty"`
	// UseReceiverType is --use-receiver-type.
	UseReceiverType *string `json:"use_receiver_type,omitempty"`
	// UseReceiverTypePackage is --use-receiver-type-package.
	UseReceiverTypePackage *string `json:"use_receiver_type_package,omitempty"`
	// UseTemplatesVariable is --use-templates-variable.
	UseTemplatesVariable []string `json:"use_templates_variable,omitempty"`
}

// read reads the muxt.json at path, an absolute path its errors name the file
// by. A missing file is an error that matches fs.ErrNotExist.
func read(path string, deprecated map[string]string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			line, col := position(data, syntax.Offset)
			return nil, &ParseError{Path: path, Line: line, Col: col, Err: syntax}
		}
		return nil, fmt.Errorf("parsing %s: %w", path, err)
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
	if err := f.checkKeys(data, deprecated); err != nil {
		return nil, err
	}
	return &f, nil
}

// checkKeys returns the checkPackageKey error for the first packages key, in
// sorted order, that is not an import path of a package at or below the file's
// directory, or the entry-key errors of checkEntryKeys for package_defaults
// and then for each packages entry. json.Unmarshal drops unknown keys without a
// word, so the keys are read again here.
func (f *File) checkKeys(data []byte, deprecated map[string]string) error {
	var raw struct {
		PackageDefaults map[string]json.RawMessage            `json:"package_defaults"`
		Packages        map[string]map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing %s: %w", f.Name, err)
	}
	known := entryKeys()
	if err := checkEntryKeys(f.Name, "package_defaults", raw.PackageDefaults, deprecated, known); err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(raw.Packages)) {
		if err := f.checkPackageKey(key); err != nil {
			return err
		}
		if err := checkEntryKeys(f.Name, fmt.Sprintf("packages[%q]", key), raw.Packages[key], deprecated, known); err != nil {
			return err
		}
	}
	return nil
}

// checkPackageKey returns a *KeyFormError for a key that is a directory
// relative to the file, or an import path of the module that is not clean, and
// a *KeyOutsideError for one that names a package outside the file's
// directory, so a checked-out muxt.json cannot point generate at, or write
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
		return nil
	}
	// A key that is not an import path of the module is either a directory
	// the way keys used to be written, which has an import path to name, or
	// the import path of some other package.
	if !isRelative(key) {
		return &KeyOutsideError{Path: f.Name, Key: key}
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
// *UnknownKeyError. known is entryKeys.
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
		return &UnknownKeyError{Path: path, Where: where, Key: key}
	}
	return nil
}

// entryKeys returns the json key of each Entry field: the keys an entry accepts.
func entryKeys() []string {
	var keys []string
	entry := reflect.TypeFor[Entry]()
	for i := range entry.NumField() {
		name, _, _ := strings.Cut(entry.Field(i).Tag.Get("json"), ",")
		keys = append(keys, name)
	}
	return keys
}

// position returns the 1-based line and column of the byte before offset. The
// json package reports an offset one past the byte it rejected.
func position(data []byte, offset int64) (line, col int) {
	pos := max(int(offset)-1, 0)
	before := data[:pos]
	return 1 + bytes.Count(before, []byte{'\n'}), pos - bytes.LastIndexByte(before, '\n')
}
