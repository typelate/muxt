package templatecover

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/typelate/htt"

	"github.com/typelate/muxt/internal/source"
)

// Source is one template file, by absolute path, and the trees parsed from
// it.
type Source struct {
	File  string
	Trees []*htt.Tree
}

// Sources reads the template files of pkg: every definition of every
// variable, shared fragments from another package's embed.FS included,
// whose file's base name is its tree's parse name. A string template or a
// file read at run time has no such definition, so none of its blocks can
// be placed in a file.
//
// Sources are sorted by file, and a tree appears once however many
// variables share it.
func Sources(pkg source.Package) []Source {
	byFile := make(map[string][]*htt.Tree)
	for _, v := range pkg.Variables {
		for _, d := range v.Definitions {
			file := d.Define.Filename
			if d.Tree == nil || file == "" || path.Base(filepath.ToSlash(file)) != d.Tree.ParseName {
				continue
			}
			if !slices.Contains(byFile[file], d.Tree) {
				byFile[file] = append(byFile[file], d.Tree)
			}
		}
	}
	sources := make([]Source, 0, len(byFile))
	for file, trees := range byFile {
		sources = append(sources, Source{File: file, Trees: trees})
	}
	slices.SortFunc(sources, func(a, b Source) int { return cmp.Compare(a.File, b.File) })
	return sources
}

// Profile is template blocks by absolute file path.
type Profile struct {
	files map[string]map[Block]bool
}

// AmbiguousNameError is a parse name that two or more template files
// share, so a block reached under it cannot be put in one of them. A parse
// name is a file's base name, and a shared fragment can have the base name
// of a local file.
type AmbiguousNameError struct {
	ParseName string
	// Files are the absolute paths that have the name, sorted.
	Files []string
}

func (e *AmbiguousNameError) Error() string {
	last := len(e.Files) - 1
	files := e.Files[0]
	if last > 0 {
		files = strings.Join(e.Files[:last], ", ") + " and " + e.Files[last]
	}
	return fmt.Sprintf("%s share the parse name %s; the cover profile leaves out its blocks", files, e.ParseName)
}

// Add attributes what was reached to the files in sources. A parse name no
// source has is dropped, and so is one that two files have, which is
// returned in parse name order.
func (p *Profile) Add(r Reached, sources []Source) []*AmbiguousNameError {
	filesByName := make(map[string][]string)
	for _, s := range sources {
		for _, tree := range s.Trees {
			if !slices.Contains(filesByName[tree.ParseName], s.File) {
				filesByName[tree.ParseName] = append(filesByName[tree.ParseName], s.File)
			}
		}
	}
	var ambiguous []*AmbiguousNameError
	for _, name := range slices.Sorted(maps.Keys(r)) {
		files := filesByName[name]
		switch len(files) {
		case 0:
		case 1:
			if p.files == nil {
				p.files = make(map[string]map[Block]bool)
			}
			if p.files[files[0]] == nil {
				p.files[files[0]] = make(map[Block]bool)
			}
			for b := range r[name] {
				p.files[files[0]][b] = true
			}
		default:
			slices.Sort(files)
			ambiguous = append(ambiguous, &AmbiguousNameError{ParseName: name, Files: files})
		}
	}
	return ambiguous
}

// WriteTo writes the profile in the format of a Go cover profile.
func (p *Profile) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, "mode: set\n")
	total := int64(n)
	if err != nil {
		return total, err
	}
	for file, blocks := range p.files {
		for b := range blocks {
			n, err := fmt.Fprintf(w, "%s:%d.%d,%d.%d 1 1\n", file, b.StartLine, b.StartCol, b.EndLine, b.EndCol)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
	}
	return total, nil
}
