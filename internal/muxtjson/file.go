// Package muxtjson reads a muxt.json file, which records the generate flags of
// package directories so a flagless muxt generate knows what to write.
package muxtjson

import (
	"encoding/json"
	"fmt"
	"os"
)

// FileName is the name of the configuration file muxt looks for.
const FileName = "muxt.json"

// File is a muxt.json. Packages is keyed by slash-separated directory,
// relative to the file, such as ".".
type File struct {
	Packages map[string]Entry `json:"packages"`
}

// Entry holds the generate flags of one package directory. A nil field is
// absent from the file, so the flag's default applies.
type Entry struct {
	UseReceiverType  *string `json:"use-receiver-type,omitempty"`
	OutputRoutesFunc *string `json:"output-routes-func,omitempty"`
}

// Read reads the muxt.json at path. A missing file is an error that matches
// fs.ErrNotExist.
func Read(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &f, nil
}
