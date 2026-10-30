package load

import (
	"fmt"
	"slices"

	"golang.org/x/tools/go/packages"
)

// EmbeddedFiles returns the absolute paths of the files the package in dir
// embeds, sorted. ok is false when dir holds no Go package.
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
	return files, true, nil
}
