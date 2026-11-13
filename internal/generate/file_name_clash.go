package generate

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/source"
)

// GeneratedFileClashError reports two template files that would write the same
// generated file with --output-multiple-files. The generated name is the
// template file's base name, so a package's layout.gohtml and the one in a
// package it shares templates from both want layout_template_routes_gen.go,
// and writing both would leave only the last.
type GeneratedFileClashError struct {
	// FileName is the generated file name both would write.
	FileName string

	// First and Second are the template files by absolute path. The package's
	// own file is First.
	First, Second string
}

func (e *GeneratedFileClashError) Error() string {
	return fmt.Sprintf("%s would be written for both %s and %s; rename one template file", e.FileName, e.First, e.Second)
}

// checkGeneratedFileNames fails when two of sourceFiles would write the same
// generated file. A file another package owns is listed after the package's
// own, so the error names the file the package can rename first.
func checkGeneratedFileNames(sourceFiles []string, pkg source.Package) error {
	shared := make(map[string]bool)
	for _, variable := range pkg.Variables {
		for _, definition := range variable.Definitions {
			if definition.Shared() && definition.Tree != nil {
				shared[definition.Tree.Filepath] = true
			}
		}
	}
	ordered := slices.Clone(sourceFiles)
	slices.SortStableFunc(ordered, func(a, b string) int {
		switch {
		case shared[a] == shared[b]:
			return strings.Compare(a, b)
		case shared[a]:
			return 1
		default:
			return -1
		}
	})
	writtenFor := make(map[string]string)
	for _, sourceFile := range ordered {
		name := generatedFileName(sourceFile)
		if first, ok := writtenFor[name]; ok {
			return &GeneratedFileClashError{FileName: name, First: first, Second: sourceFile}
		}
		writtenFor[name] = sourceFile
	}
	return nil
}

// generatedFileName is the file --output-multiple-files writes the routes of
// sourceFile into. Only the base name is used, since sourceFile may be an
// absolute path.
func generatedFileName(sourceFile string) string {
	return strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile)) + "_template_routes_gen.go"
}
