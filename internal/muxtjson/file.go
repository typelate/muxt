// Package muxtjson reads a muxt.json file, which records the generate flags of
// package directories so a flagless muxt generate knows what to write.
package muxtjson

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

// File is a muxt.json. Dir is the directory holding the file, set by Read.
// Packages is keyed by slash-separated directory, relative to Dir, such as ".".
type File struct {
	// Dir is the absolute directory holding the file, so package directories
	// can be found from it.
	Dir string `json:"-"`
	// Name is the file's path relative to the working directory the search
	// started from. Errors name the file this way.
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
// its replacement, as Read takes it.
func Find(wd string, deprecated map[string]string) (f *File, found bool, err error) {
	start, err := filepath.Abs(wd)
	if err != nil {
		return nil, false, fmt.Errorf("absolute path of %s: %w", wd, err)
	}
	dir := start
	for {
		// Errors name the file relative to wd, the way the rest of muxt
		// names paths, while Dir keeps the absolute directory.
		shown, err := filepath.Rel(start, filepath.Join(dir, FileName))
		if err != nil {
			return nil, false, fmt.Errorf("path of %s from %s: %w", dir, start, err)
		}
		f, err := Read(filepath.Join(dir, FileName), shown, deprecated)
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

// KeyFor returns the slash-separated path of dir relative to the directory
// holding f, the key its Packages entry uses. The directory itself is ".".
func (f *File) KeyFor(dir string) (string, error) {
	rel, err := filepath.Rel(f.Dir, dir)
	if err != nil {
		return "", fmt.Errorf("key for %s: %w", dir, err)
	}
	return filepath.ToSlash(rel), nil
}

// Keys returns the packages keys sorted. Read rejects a key not in clean form,
// so the keys are the ones a person wrote.
func (f *File) Keys() []string {
	return slices.Sorted(maps.Keys(f.Packages))
}

// DirOf returns the absolute directory of a packages key.
func (f *File) DirOf(key string) string {
	return filepath.Join(f.Dir, filepath.FromSlash(key))
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

// Read reads the muxt.json at path. shown is the name its errors give the
// file, relative to the working directory, so a message names the file the
// way the person typed it. A missing file is an error that matches
// fs.ErrNotExist.
func Read(path, shown string, deprecated map[string]string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", shown, err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			line, col := position(data, syntax.Offset)
			return nil, &ParseError{Path: shown, Line: line, Col: col, Err: syntax}
		}
		return nil, fmt.Errorf("parsing %s: %w", shown, err)
	}
	if err := checkKeys(shown, data, deprecated); err != nil {
		return nil, err
	}
	f.Dir = filepath.Dir(path)
	f.Name = shown
	return &f, nil
}

// checkKeys returns a *KeyFormError for the first packages key, in sorted
// order, that is not in clean form, a *KeyOutsideError for one that leaves the
// file's directory, or the entry-key errors of checkEntryKeys for package_defaults
// and then for each packages entry. json.Unmarshal drops unknown keys without a
// word, so the keys are read again here.
func checkKeys(shown string, data []byte, deprecated map[string]string) error {
	var raw struct {
		PackageDefaults map[string]json.RawMessage            `json:"package_defaults"`
		Packages        map[string]map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing %s: %w", shown, err)
	}
	known := entryKeys()
	if err := checkEntryKeys(shown, "package_defaults", raw.PackageDefaults, deprecated, known); err != nil {
		return err
	}
	for _, dir := range slices.Sorted(maps.Keys(raw.Packages)) {
		// Two spellings of one directory would be two entries, so each key
		// is written in the form path.Clean gives it.
		if clean := path.Clean(dir); clean != dir {
			return &KeyFormError{Path: shown, Key: dir, Clean: clean}
		}
		// A key is a directory below the file's own, so a checked-out
		// muxt.json cannot point generate at, or write into, another place.
		if dir != "." && !filepath.IsLocal(filepath.FromSlash(dir)) {
			return &KeyOutsideError{Path: shown, Key: dir}
		}
		if err := checkEntryKeys(shown, fmt.Sprintf("packages[%q]", dir), raw.Packages[dir], deprecated, known); err != nil {
			return err
		}
	}
	return nil
}

// checkEntryKeys returns the error for the first key, in sorted order, of the
// entry where names: a *DeprecatedKeyError, a *DashedKeyError, or an
// *UnknownKeyError. known is entryKeys.
func checkEntryKeys(shown, where string, keys map[string]json.RawMessage, deprecated map[string]string, known []string) error {
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if replacement, ok := deprecated[key]; ok {
			return &DeprecatedKeyError{Path: shown, Where: where, Key: key, Replacement: replacement}
		}
		if slices.Contains(known, key) {
			continue
		}
		// The dashed spelling is named only when the underscore form is a
		// key, so a typo with dashes still reads as an unknown key.
		if underscored := strings.ReplaceAll(key, "-", "_"); slices.Contains(known, underscored) {
			return &DashedKeyError{Path: shown, Where: where, Key: key, Underscored: underscored}
		}
		return &UnknownKeyError{Path: shown, Where: where, Key: key}
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
