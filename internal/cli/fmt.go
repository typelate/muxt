package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/diff"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format"
	"github.com/typelate/muxt/internal/template/lex"
)

const standardInputName = "<standard input>"

// errFormatFailed is what muxt fmt returns once it has reported its
// problems on stderr, or listed or diffed a file that differs; the error
// itself is not printed.
var errFormatFailed = errors.New("fmt failed")

// formatFile is one file muxt fmt formats.
type formatFile struct {
	name   string // as given or as walked; how the file is reported
	path   string // where it is read and written; "" for standard input
	dir    string // the directory whose package the file belongs to
	source []byte
	tree   *template.Tree // nil when the file does not parse
}

// runFormatCommand formats the files config names, or standard input, as
// gofmt does. Problems with a file are reported on stderr, one line each,
// and the other files are still formatted.
func runFormatCommand(cmd *cobra.Command, wd string, config formatConfiguration) error {
	r := formatRun{config: config, stdout: cmd.OutOrStdout(), stderr: cmd.ErrOrStderr(), packages: make(map[string]formatPackage)}
	var files []*formatFile
	if len(config.Paths) == 0 {
		source, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("reading standard input: %w", err)
		}
		files = append(files, &formatFile{name: standardInputName, dir: wd, source: source})
	} else {
		files = r.readFiles(wd, config.Paths)
	}
	for _, f := range files {
		f.tree = parseFormatTree(f.name, f.source)
	}
	for _, f := range files {
		r.format(f, files)
	}
	if r.failed {
		cmd.SilenceErrors = true
		return errFormatFailed
	}
	return nil
}

type formatRun struct {
	config         formatConfiguration
	stdout, stderr io.Writer
	packages       map[string]formatPackage
	failed         bool
}

// formatPackage is the Go package in a directory, as muxt fmt sees it:
// whether there is one and the templates it embeds.
type formatPackage struct {
	loaded   bool
	embedded []*template.Tree
	paths    map[string]bool
}

func (r *formatRun) report(message string) {
	r.failed = true
	_, _ = fmt.Fprintln(r.stderr, message)
}

// readFiles reads the files paths name, walking directories for
// *.gohtml.
func (r *formatRun) readFiles(wd string, paths []string) []*formatFile {
	var files []*formatFile
	add := func(name string) {
		path := absoluteDir(wd, name)
		source, err := os.ReadFile(path)
		if err != nil {
			r.report(err.Error())
			return
		}
		files = append(files, &formatFile{name: name, path: path, dir: filepath.Dir(path), source: source})
	}
	for _, name := range paths {
		info, err := os.Stat(absoluteDir(wd, name))
		if err != nil || !info.IsDir() {
			add(name)
			continue
		}
		err = filepath.WalkDir(absoluteDir(wd, name), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				r.report(err.Error())
				return nil
			}
			rel, err := filepath.Rel(absoluteDir(wd, name), path)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if rel != "." && skipFormatDirectory(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) == ".gohtml" {
				add(filepath.Join(name, rel))
			}
			return nil
		})
		if err != nil {
			r.report(err.Error())
		}
	}
	return files
}

// skipFormatDirectory reports whether a directory walked for templates is
// left out, as the go command leaves it out of ./... patterns.
func skipFormatDirectory(name string) bool {
	return name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// format formats f, with every file of the run and the templates f's
// package embeds as context for its author CSS, and writes the result
// where config says.
func (r *formatRun) format(f *formatFile, all []*formatFile) {
	pkg := r.packageIn(f.dir)
	var context []*template.Tree
	for _, other := range all {
		if other != f && other.tree != nil && !pkg.paths[other.path] {
			context = append(context, other.tree)
		}
	}
	for _, tree := range pkg.embedded {
		if tree.ParseName != f.path {
			context = append(context, tree)
		}
	}
	var files []*template.Tree
	if f.tree != nil {
		files = append(files, f.tree)
	}
	styles, err := formatStyles(f.dir, pkg.loaded, files, context)
	if err != nil {
		r.report(fmt.Sprintf("%s: %v", f.name, err))
		return
	}
	out, err := format.Source(f.name, string(f.source), format.Styles(styles))
	if err != nil {
		r.report(formatErrorLine(f.name, err))
		return
	}
	r.write(f, []byte(out))
}

func formatErrorLine(name string, err error) string {
	var parseErr *template.ParseError
	if errors.As(err, &parseErr) {
		pos := parseErr.Position()
		return fmt.Sprintf("%s:%d:%d: %v", name, pos.Line, pos.Column, errors.Unwrap(parseErr))
	}
	return err.Error()
}

func (r *formatRun) write(f *formatFile, out []byte) {
	changed := !bytes.Equal(f.source, out)
	if changed && (r.config.List || r.config.Diff) {
		r.failed = true
	}
	if changed && r.config.List {
		_, _ = fmt.Fprintln(r.stdout, f.name)
	}
	if changed && r.config.Write {
		if err := writeFileAtomic(f.path, out); err != nil {
			r.report(err.Error())
		}
	}
	if changed && r.config.Diff {
		_, _ = r.stdout.Write(diff.Diff(f.name+".orig", f.source, f.name, out))
	}
	if !r.config.List && !r.config.Write && !r.config.Diff {
		_, _ = r.stdout.Write(out)
	}
}

// packageIn loads the Go package in dir once per run. A directory outside
// any module, or holding no Go files, has no package.
func (r *formatRun) packageIn(dir string) formatPackage {
	if pkg, ok := r.packages[dir]; ok {
		return pkg
	}
	var pkg formatPackage
	embedded, ok, err := load.EmbeddedFiles(dir)
	if err == nil && ok {
		pkg = formatPackage{loaded: true, paths: make(map[string]bool)}
		for _, path := range embedded {
			if filepath.Ext(path) != ".gohtml" {
				continue
			}
			pkg.paths[path] = true
			source, err := os.ReadFile(path)
			if err != nil {
				continue // an embedded file fmt cannot read is no context; go build reports it
			}
			if tree := parseFormatTree(path, source); tree != nil {
				pkg.embedded = append(pkg.embedded, tree)
			}
		}
	}
	r.packages[dir] = pkg
	return pkg
}

// parseFormatTree parses source as format.Source does, for its links and
// <style> elements. It returns nil when source does not parse; format
// reports that for a file being formatted.
func parseFormatTree(name string, source []byte) *template.Tree {
	tree, err := template.ParseTree(name, name, string(source), nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
		LeftDelim:   "{{",
		RightDelim:  "}}",
	})
	if err != nil {
		return nil
	}
	return tree
}

// writeFileAtomic replaces the file at path with data, keeping its
// permissions: data goes to a temporary file in the same directory, which
// is then renamed over path.
func writeFileAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".muxt-fmt-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return errors.Join(err, tmp.Close(), os.Remove(tmp.Name()))
	}
	if err := tmp.Close(); err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}
	return nil
}
