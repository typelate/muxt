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
	Dir      string           `json:"-"`
	Packages map[string]Entry `json:"packages"`
}

// Find reads the first muxt.json found walking up from wd, starting at wd
// itself. The walk stops at the module root: the first directory holding a
// go.mod is checked for muxt.json and no directory above it is. A muxt.json
// above the module belongs to another project, so it is not read. When no
// go.mod is found on the way up, the walk goes to the filesystem root. found is
// false when the walk ends without a muxt.json. deprecated maps an old key to
// its replacement, as Read takes it.
func Find(wd string, deprecated map[string]string) (f *File, found bool, err error) {
	dir, err := filepath.Abs(wd)
	if err != nil {
		return nil, false, fmt.Errorf("absolute path of %s: %w", wd, err)
	}
	for {
		f, err := Read(filepath.Join(dir, FileName), deprecated)
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

// Entry holds the generate flags of one package directory. JSON keys are the
// flag names without "--". A nil field is absent from the file, so the flag's
// default applies. Fields follow the order `muxt generate --help` lists flags;
// verbose is not configuration and has no field.
type Entry struct {
	// InputStaticAssetsDirectory is --input-static-assets-directory.
	InputStaticAssetsDirectory *string `json:"input-static-assets-directory,omitempty"`
	// OutputDatastar is --output-datastar.
	OutputDatastar *bool `json:"output-datastar,omitempty"`
	// OutputExportedDefaultIdentifiers is --output-exported-default-identifiers.
	OutputExportedDefaultIdentifiers *bool `json:"output-exported-default-identifiers,omitempty"`
	// OutputFile is --output-file.
	OutputFile *string `json:"output-file,omitempty"`
	// OutputHTMX is --output-htmx.
	OutputHTMX *bool `json:"output-htmx,omitempty"`
	// OutputMultipartMaxMemory is --output-multipart-max-memory, in the form the flag accepts, such as "64MB".
	OutputMultipartMaxMemory *string `json:"output-multipart-max-memory,omitempty"`
	// OutputMultipleFiles is --output-multiple-files.
	OutputMultipleFiles *bool `json:"output-multiple-files,omitempty"`
	// OutputMuxtVersion is --output-muxt-version.
	OutputMuxtVersion *bool `json:"output-muxt-version,omitempty"`
	// OutputReceiverInterface is --output-receiver-interface.
	OutputReceiverInterface *string `json:"output-receiver-interface,omitempty"`
	// OutputRoutesFunc is --output-routes-func.
	OutputRoutesFunc *string `json:"output-routes-func,omitempty"`
	// OutputRoutesFuncWithLoggerParam is --output-routes-func-with-logger-param.
	OutputRoutesFuncWithLoggerParam *bool `json:"output-routes-func-with-logger-param,omitempty"`
	// OutputRoutesFuncWithMiddlewareParam is --output-routes-func-with-middleware-param.
	OutputRoutesFuncWithMiddlewareParam *bool `json:"output-routes-func-with-middleware-param,omitempty"`
	// OutputRoutesFuncWithPathPrefixParam is --output-routes-func-with-path-prefix-param.
	OutputRoutesFuncWithPathPrefixParam *bool `json:"output-routes-func-with-path-prefix-param,omitempty"`
	// OutputSSETemplateDataType is --output-sse-template-data-type.
	OutputSSETemplateDataType *string `json:"output-sse-template-data-type,omitempty"`
	// OutputStaticAssetsPath is --output-static-assets-path.
	OutputStaticAssetsPath *string `json:"output-static-assets-path,omitempty"`
	// OutputTemplateDataType is --output-template-data-type.
	OutputTemplateDataType *string `json:"output-template-data-type,omitempty"`
	// OutputTemplateRouteBuilderType is --output-template-route-builder-type.
	OutputTemplateRouteBuilderType *string `json:"output-template-route-builder-type,omitempty"`
	// OutputTemplateRoutePathsType is --output-template-route-paths-type.
	OutputTemplateRoutePathsType *string `json:"output-template-route-paths-type,omitempty"`
	// OutputTemplateRouteType is --output-template-route-type.
	OutputTemplateRouteType *string `json:"output-template-route-type,omitempty"`
	// UseReceiverType is --use-receiver-type.
	UseReceiverType *string `json:"use-receiver-type,omitempty"`
	// UseReceiverTypePackage is --use-receiver-type-package.
	UseReceiverTypePackage *string `json:"use-receiver-type-package,omitempty"`
	// UseTemplatesVariable is --use-templates-variable.
	UseTemplatesVariable []string `json:"use-templates-variable,omitempty"`
}

// Read reads the muxt.json at path. A missing file is an error that matches
// fs.ErrNotExist.
func Read(path string, deprecated map[string]string) (*File, error) {
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
	if err := checkKeys(path, data, deprecated); err != nil {
		return nil, err
	}
	f.Dir = filepath.Dir(path)
	return &f, nil
}

// checkKeys returns a *DeprecatedKeyError or *UnknownKeyError for the first
// entry key, in sorted order, that is an old flag name or that Entry has no
// field for. json.Unmarshal drops unknown keys without a word, so the keys are
// read again here.
func checkKeys(path string, data []byte, deprecated map[string]string) error {
	var raw struct {
		Packages map[string]map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	known := entryKeys()
	for _, dir := range slices.Sorted(maps.Keys(raw.Packages)) {
		where := fmt.Sprintf("packages[%q]", dir)
		for _, key := range slices.Sorted(maps.Keys(raw.Packages[dir])) {
			if replacement, ok := deprecated[key]; ok {
				return &DeprecatedKeyError{Path: path, Where: where, Key: key, Replacement: replacement}
			}
			if !slices.Contains(known, key) {
				return &UnknownKeyError{Path: path, Where: where, Key: key}
			}
		}
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
