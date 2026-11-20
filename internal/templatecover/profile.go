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

// Profile is template blocks by absolute file path, each true once a test
// reached it.
type Profile struct {
	files map[string]map[Block]bool
}

// record puts b in file, reached or not. A block some test reached stays
// reached: a fragment two packages share is covered when either's tests
// run it.
func (p *Profile) record(file string, b Block, reached bool) {
	if p.files == nil {
		p.files = make(map[string]map[Block]bool)
	}
	if p.files[file] == nil {
		p.files[file] = make(map[Block]bool)
	}
	p.files[file][b] = p.files[file][b] || reached
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

// Add attributes what was reached to the files in sources, and gives every
// other block of those files a line that says no test reached it. A parse
// name no source has is dropped. One that two files have is returned in
// parse name order, and no block of either file is written for it: which
// of them was reached is unknown.
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
	leftOut := make(map[string]bool)
	for _, name := range slices.Sorted(maps.Keys(r)) {
		files := filesByName[name]
		switch len(files) {
		case 0:
		case 1:
			for b := range r[name] {
				p.record(files[0], b, true)
			}
		default:
			slices.Sort(files)
			ambiguous = append(ambiguous, &AmbiguousNameError{ParseName: name, Files: files})
			leftOut[name] = true
		}
	}
	for _, s := range sources {
		for _, tree := range s.Trees {
			if leftOut[tree.ParseName] {
				continue
			}
			for _, b := range tree.CoverBlocks() {
				p.record(s.File, Block{StartLine: b.StartLine, StartCol: b.StartColumn, EndLine: b.EndLine, EndCol: b.EndColumn}, false)
			}
		}
	}
	return ambiguous
}

// column is col as go tool cover reads it. x/tools cover.Boundaries starts
// counting columns on line 1 at 2, not 1, so without this a span on line 1
// would start one byte early.
func column(line, col int) int {
	if line == 1 {
		return col + 1
	}
	return col
}

// WriteTo writes the profile in the format of a Go cover profile.
func (p *Profile) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, "mode: set\n")
	total := int64(n)
	if err != nil {
		return total, err
	}
	for _, file := range slices.Sorted(maps.Keys(p.files)) {
		blocks := slices.SortedFunc(maps.Keys(p.files[file]), func(a, b Block) int {
			return cmp.Or(
				cmp.Compare(a.StartLine, b.StartLine), cmp.Compare(a.StartCol, b.StartCol),
				cmp.Compare(a.EndLine, b.EndLine), cmp.Compare(a.EndCol, b.EndCol))
		})
		for _, b := range blocks {
			count := 0
			if p.files[file][b] {
				count = 1
			}
			n, err := fmt.Fprintf(w, "%s:%d.%d,%d.%d 1 %d\n", file, b.StartLine, column(b.StartLine, b.StartCol), b.EndLine, column(b.EndLine, b.EndCol), count)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
	}
	return total, nil
}
