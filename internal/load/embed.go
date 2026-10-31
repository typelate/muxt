package load

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// EmbeddedFiles returns the absolute paths of the files the package in dir
// embeds, sorted. ok is false when dir holds no Go package, or the go
// command could not load one there (dir is outside any module); err then
// says why the load failed. When ok and err is not nil, the package loaded
// with errors (a go:embed pattern that matches nothing, say): files may
// miss some it embeds. err joins one *PackageError for each error.
func EmbeddedFiles(dir string) (files []string, ok bool, err error) {
	cfg := &packages.Config{
		Dir:  dir,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedEmbedFiles,
	}
	pl, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, false, fmt.Errorf("loading package in %s: %w", dir, err)
	}
	if len(pl) == 0 || len(pl[0].GoFiles) == 0 && len(pl[0].EmbedFiles) == 0 {
		return nil, false, nil
	}
	files = slices.Clone(pl[0].EmbedFiles)
	slices.Sort(files)
	var loadErrs []error
	for _, e := range pl[0].Errors {
		loadErrs = append(loadErrs, packageError(dir, e))
	}
	return files, true, errors.Join(loadErrs...)
}

// PackageError is an error the go command reported for a package.
type PackageError struct {
	File     string // absolute; "" when the error has no position
	Position string // the line and column after File, as ":5:21"
	Msg      string
}

func (e *PackageError) Error() string {
	if e.File == "" {
		return e.Msg
	}
	return e.File + e.Position + ": " + e.Msg
}

// packageError is e with its file, which the go command gives relative to
// the package directory, made absolute.
func packageError(dir string, e packages.Error) error {
	if e.Pos == "" || e.Pos == "-" {
		return &PackageError{Msg: e.Msg}
	}
	file, position := splitPosition(e.Pos)
	if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	return &PackageError{File: file, Position: position, Msg: e.Msg}
}

// splitPosition splits "file:line:col" (or "file:line") into the file and
// the ":line:col" after it.
func splitPosition(pos string) (file, position string) {
	file = pos
	for range 2 {
		i := strings.LastIndexByte(file, ':')
		if i < 0 || !isDigits(file[i+1:]) {
			break
		}
		file = file[:i]
	}
	return file, pos[len(file):]
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
