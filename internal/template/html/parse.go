package html

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// ParseFiles creates a new Template and parses the template definitions from
// the named files. The returned template's name will have the base name and
// parsed contents of the first file. There must be at least one file.
func ParseFiles(filenames ...string) (*Template, error) {
	return parseFiles(nil, readFileOS, filenames...)
}

// ParseGlob creates a new Template and parses the template definitions from
// the files identified by the pattern. The pattern is processed by
// filepath.Glob and must match at least one file.
func ParseGlob(pattern string) (*Template, error) {
	return parseGlob(nil, pattern)
}

// ParseFS is like ParseFiles or ParseGlob but reads from the file system fsys
// instead of the host operating system's file system.
func ParseFS(fsys fs.FS, patterns ...string) (*Template, error) {
	return parseFS(nil, fsys, patterns...)
}

func parseFiles(t *Template, readFile func(string) (string, string, []byte, error), filenames ...string) (*Template, error) {
	if len(filenames) == 0 {
		return nil, fmt.Errorf("html/template: no files named in call to ParseFiles")
	}
	for _, filename := range filenames {
		name, fullPath, b, err := readFile(filename)
		if err != nil {
			return nil, err
		}
		s := string(b)
		var tmpl *Template
		if t == nil {
			t = New(name)
			tmpl = t
		} else {
			tmpl = t.New(name)
		}
		_, err = tmpl.Parse(s)
		if err != nil {
			return nil, err
		}
		if fullPath != "" {
			tmpl.Tree.Filepath = fullPath
			for _, tr := range t.common.tmpl {
				if tr.Tree != nil && tr.Tree.Filepath == "" && tr.Tree.ParseName == tmpl.Tree.ParseName {
					tr.Tree.Filepath = fullPath
				}
			}
		}
	}
	return t, nil
}

func parseGlob(t *Template, pattern string) (*Template, error) {
	filenames, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(filenames) == 0 {
		return nil, fmt.Errorf("html/template: pattern matches no files: %#q", pattern)
	}
	return parseFiles(t, readFileOS, filenames...)
}

func parseFS(t *Template, fsys fs.FS, patterns ...string) (*Template, error) {
	var filenames []string
	for _, pattern := range patterns {
		list, err := fs.Glob(fsys, pattern)
		if err != nil {
			return nil, err
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("html/template: pattern matches no files: %#q", pattern)
		}
		filenames = append(filenames, list...)
	}
	return parseFiles(t, func(file string) (string, string, []byte, error) {
		name := path.Base(file)
		b, err := fs.ReadFile(fsys, file)
		return name, "", b, err
	}, filenames...)
}

func readFileOS(file string) (string, string, []byte, error) {
	name := filepath.Base(file)
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", "", nil, err
	}
	b, err := os.ReadFile(file)
	return name, abs, b, err
}
