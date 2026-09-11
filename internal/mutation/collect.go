package mutation

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"
)

// sourceKey identifies the text a template was written in: a template
// file, or one string literal within a Go file.
type sourceKey struct {
	file     string
	litStart int
}

// sourceCollector turns the definitions a template set reports into the
// distinct texts holding them, reading each file once.
type sourceCollector struct {
	workingDirectory string
	packages         []*packages.Package
	files            map[string]string
	byKey            map[sourceKey]*templateSource
	keys             []sourceKey

	// delims are the delimiters each source was parsed with, read off the
	// definitions before any source is built. A source scans its actions
	// as it is constructed, so the delimiters have to be known by then,
	// and the definition that reveals them is not necessarily the first
	// one filed for that source.
	delims map[sourceKey][2]string
}

// resolveDelimiters reads the delimiters each source was parsed with off
// the definitions written in it.
//
// They are keyed by source rather than by file because a construction
// chain may call Delims more than once, and one Go file can hold several
// parsed literals. Keying by file would give every literal in a file the
// pair of whichever definition was seen first, and the rest would be read
// with delimiters they were not written in -- which yields a tree with no
// actions in it and a template that silently contributes no mutants.
//
// Within one source the pair is fixed, so the first definition that
// reveals it answers for the whole source. A source whose only template
// has no define clause reveals nothing and keeps the defaults.
func (c *sourceCollector) resolveDelimiters(defs []check.Definition) {
	for _, definition := range defs {
		file := definition.Define.Position.Filename
		if file == "" {
			continue
		}
		key, ok := c.keyFor(definition)
		if !ok {
			continue
		}
		if _, known := c.delims[key]; known {
			continue
		}
		text, err := c.read(file)
		if err != nil {
			continue
		}
		if left, right, ok := delimiters(text, definition); ok {
			c.delims[key] = [2]string{left, right}
		}
	}
}

// keyFor names the source a definition was written in.
//
// It is the same key add files the definition under, so the delimiters
// resolved here reach the source they were read from.
func (c *sourceCollector) keyFor(definition check.Definition) (sourceKey, bool) {
	file := definition.Define.Position.Filename
	if filepath.Ext(file) != ".go" {
		return sourceKey{file: file}, true
	}
	litStart, _, ok := findStringLiteral(c.packages, file, definition.Define.Offset)
	if !ok {
		return sourceKey{}, false
	}
	return sourceKey{file: file, litStart: litStart}, true
}

func newSourceCollector(workingDirectory string, pl []*packages.Package, defs []check.Definition) *sourceCollector {
	c := &sourceCollector{
		workingDirectory: workingDirectory,
		packages:         pl,
		files:            make(map[string]string),
		byKey:            make(map[sourceKey]*templateSource),
		delims:           make(map[sourceKey][2]string),
	}
	c.resolveDelimiters(defs)
	return c
}

// delimitersFor reports the delimiters a source was parsed with, empty
// for the text/template defaults.
func (c *sourceCollector) delimitersFor(key sourceKey) (string, string) {
	pair := c.delims[key]
	return pair[0], pair[1]
}

// add files one definition under the text it was written in, reading
// that text at most once, and reports the source it was filed under.
//
// A definition the collector cannot place -- a Go string literal it has
// no package for -- is reported as no source rather than as an error,
// since the caller may hold others it can still use.
func (c *sourceCollector) add(definition check.Definition) (*templateSource, error) {
	file := definition.Define.Position.Filename
	if file == "" {
		return nil, nil
	}
	fileText, err := c.read(file)
	if err != nil {
		return nil, err
	}

	var src *templateSource
	if filepath.Ext(file) != ".go" {
		left, right := c.delimitersFor(sourceKey{file: file})
		src, err = c.source(sourceKey{file: file}, func() (*templateSource, error) {
			return newFileSource(file, c.relative(file), fileText, left, right), nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		litStart, litEnd, ok := findStringLiteral(c.packages, file, definition.Define.Offset)
		if !ok {
			return nil, nil
		}
		left, right := c.delimitersFor(sourceKey{file: file, litStart: litStart})
		src, err = c.source(sourceKey{file: file, litStart: litStart}, func() (*templateSource, error) {
			return newLiteralSource(file, c.relative(file), definition.Name, fileText, left, right, litStart, litEnd)
		})
		if err != nil {
			return nil, err
		}
		if !definition.TemplateName.IsValid() {
			// A definition with no define clause is the template the
			// literal's own text carries, so its name is the root name
			// the text has to be parsed under.
			src.rootName = definition.Name
		}
	}

	return src, nil
}

func (c *sourceCollector) source(key sourceKey, build func() (*templateSource, error)) (*templateSource, error) {
	if existing, ok := c.byKey[key]; ok {
		return existing, nil
	}
	src, err := build()
	if err != nil {
		return nil, err
	}
	c.byKey[key] = src
	c.keys = append(c.keys, key)
	return src, nil
}

func (c *sourceCollector) read(file string) (string, error) {
	if text, ok := c.files[file]; ok {
		return text, nil
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	c.files[file] = string(b)
	return string(b), nil
}

func (c *sourceCollector) relative(file string) string {
	path, err := filepath.Rel(c.workingDirectory, file)
	if err != nil {
		return file
	}
	return filepath.ToSlash(path)
}

// sorted returns the collected sources in a stable order, so that two
// runs over an unchanged project read the same way.
func (c *sourceCollector) sorted() []*templateSource {
	keys := slices.Clone(c.keys)
	slices.SortFunc(keys, func(a, b sourceKey) int {
		return cmp.Or(
			cmp.Compare(a.file, b.file),
			cmp.Compare(a.litStart, b.litStart),
		)
	})
	sources := make([]*templateSource, 0, len(keys))
	for _, key := range keys {
		sources = append(sources, c.byKey[key])
	}
	return sources
}
