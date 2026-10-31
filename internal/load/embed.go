package load

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"golang.org/x/tools/go/packages"
)

// EmbeddedFiles returns the absolute paths of the files the package in dir
// embeds, sorted. ok is false when dir holds no Go package, or the go
// command could not load one there (dir is outside any module); err then
// says why the load failed. When ok and err is not nil, the package loaded
// with errors (a go:embed pattern that matches nothing, say): files may
// miss some it embeds. Each line of err is one error, as
// "file:line:col: message" with an absolute file when it has a position.
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

// packageError is e with its position, which the go command gives relative
// to the package directory, made absolute.
func packageError(dir string, e packages.Error) error {
	if e.Pos == "" || e.Pos == "-" {
		return errors.New(e.Msg)
	}
	pos := e.Pos
	if !filepath.IsAbs(pos) {
		pos = filepath.Join(dir, pos)
	}
	return fmt.Errorf("%s: %s", pos, e.Msg)
}
