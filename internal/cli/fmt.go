package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
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
	key    string // path with symbolic links resolved, to match the files a package embeds
	dir    string // the directory whose package the file belongs to; "" for standard input, which has none
	source []byte
	tree   *template.Tree // nil when the file does not parse
}

// runFormatCommand formats the files config names, or standard input, as
// gofmt does. Problems with a file are reported on stderr, one line each,
// and the other files are still formatted.
func runFormatCommand(cmd *cobra.Command, wd string, config formatConfiguration) error {
	r := formatRun{wd: wd, config: config, stdout: cmd.OutOrStdout(), stderr: cmd.ErrOrStderr(), packages: make(map[string]formatPackage), unread: make(map[string]bool)}
	var files []*formatFile
	if len(config.Paths) == 0 {
		source, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("reading standard input: %w", err)
		}
		// An editor pipes a buffer from wherever it runs, often the module
		// root: the package there is not the buffer's, and its CSS would be
		// worse than none.
		files = append(files, &formatFile{name: standardInputName, source: source})
	} else {
		files = r.readFiles(wd, config.Paths)
	}
	for _, f := range files {
		f.tree, _ = parseFormatTree(f.name, f.source) // format.Source reports the error
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
	wd             string
	config         formatConfiguration
	stdout, stderr io.Writer
	packages       map[string]formatPackage
	unread         map[string]bool // keys of run files that could not be read, already reported
	failed         bool
}

// formatPackage is the Go package in a directory, as muxt fmt sees it:
// whether there is one, the templates it embeds, and whether one of them
// could not be read or parsed.
type formatPackage struct {
	loaded   bool
	broken   bool
	embedded []embeddedTemplate
	keys     map[string]bool
}

type embeddedTemplate struct {
	key  string
	tree *template.Tree
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
			r.unread[resolvedPath(path)] = true
			r.report(err.Error())
			return
		}
		files = append(files, &formatFile{name: name, path: path, key: resolvedPath(path), dir: filepath.Dir(path), source: source})
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
			// As gofmt, a walk formats regular files only: a symbolic link is
			// formatted, and written through, only when it is named.
			if d.Type().IsRegular() && filepath.Ext(path) == ".gohtml" && !strings.HasPrefix(d.Name(), ".") {
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

// format formats f, with the run's other files in f's directory and the
// templates f's package embeds as context for its author CSS, and writes
// the result where config says. A file in another directory is in another
// package, whose CSS does not reach f's. When a template of the package
// cannot be read or parsed, or the package loads with errors, it may link
// or hold any stylesheet, so f is formatted as with a link that does not
// resolve: every element may have unknown CSS.
func (r *formatRun) format(f *formatFile, all []*formatFile) {
	pkg := r.packageIn(f.dir, all)
	contextBroken := pkg.broken
	var context []*template.Tree
	for _, other := range all {
		if other == f || other.dir != f.dir || pkg.keys[other.key] {
			continue
		}
		if other.tree == nil {
			if pkg.loaded {
				contextBroken = true
			}
			continue
		}
		context = append(context, other.tree)
	}
	for _, e := range pkg.embedded {
		if e.key != f.key {
			context = append(context, e.tree)
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
	styles.MissingSheets = styles.MissingSheets || contextBroken
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

// packageIn loads the Go package in dir once per run. Standard input
// (dir ""), a directory outside any module, and one holding no Go files
// have no package. A package that loads with errors is broken, and its
// errors are reported here. So is an embedded template that cannot be
// read or parsed, reported here unless it is one of the run's files,
// which report their own errors.
func (r *formatRun) packageIn(dir string, all []*formatFile) formatPackage {
	if pkg, ok := r.packages[dir]; ok {
		return pkg
	}
	var pkg formatPackage
	if dir == "" {
		r.packages[dir] = pkg
		return pkg
	}
	embedded, ok, err := load.EmbeddedFiles(dir)
	if ok {
		pkg = formatPackage{loaded: true, keys: make(map[string]bool)}
		if err != nil {
			pkg.broken = true
			r.reportLoadError(err)
		}
		assetDirs, err := staticAssetsDirectories(dir)
		if err != nil {
			// formatStyles reports it for each file of the package, which
			// it leaves unformatted.
			pkg.broken = true
		}
		runFiles := maps.Clone(r.unread)
		for _, f := range all {
			runFiles[f.key] = true
		}
		for _, path := range embedded {
			if !templateExtensions[filepath.Ext(path)] || inAnyDirectory(path, assetDirs) {
				continue
			}
			key := resolvedPath(path)
			pkg.keys[key] = true
			source, err := os.ReadFile(path)
			if err != nil {
				pkg.broken = true
				if !runFiles[key] {
					r.report(err.Error())
				}
				continue
			}
			name := r.displayName(path)
			tree, err := parseFormatTree(name, source)
			if err != nil {
				pkg.broken = true
				if !runFiles[key] {
					r.report(formatErrorLine(name, err))
				}
				continue
			}
			pkg.embedded = append(pkg.embedded, embeddedTemplate{key: key, tree: tree})
		}
	}
	r.packages[dir] = pkg
	return pkg
}

// reportLoadError reports each error of a package load on its own line,
// its file relative to the working directory as a parse error's is.
func (r *formatRun) reportLoadError(err error) {
	errs := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs = joined.Unwrap()
	}
	for _, e := range errs {
		var pkgErr *load.PackageError
		if errors.As(e, &pkgErr) && pkgErr.File != "" {
			r.report(fmt.Sprintf("%s%s: %s", r.displayName(pkgErr.File), pkgErr.Position, pkgErr.Msg))
			continue
		}
		r.report(e.Error())
	}
}

// staticAssetsDirectories are the static assets directories the generated
// files in dir record, with symbolic links resolved. The static route
// serves the files under them as they are: they are not templates.
func staticAssetsDirectories(dir string) ([]string, error) {
	configs, err := recordedStaticAssets(dir)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, config := range configs {
		dirs = append(dirs, resolvedPath(filepath.Join(dir, filepath.FromSlash(config.StaticAssetsDirectory))))
	}
	return dirs, nil
}

// inAnyDirectory reports whether path, with symbolic links resolved, is
// under one of dirs.
func inAnyDirectory(path string, dirs []string) bool {
	path = resolvedPath(path)
	for _, d := range dirs {
		rel, err := filepath.Rel(d, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// templateExtensions are the extensions of the embedded files read as
// templates for their links and <style> elements. Any other embedded file
// is not a template.
var templateExtensions = map[string]bool{".gohtml": true, ".html": true, ".htm": true, ".tmpl": true, ".gotmpl": true}

// displayName is path relative to the working directory when it is
// inside it, as a walked file is reported, and path otherwise.
func (r *formatRun) displayName(path string) string {
	rel, err := filepath.Rel(resolvedPath(r.wd), resolvedPath(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}

// resolvedPath is path with its symbolic links resolved, or path when
// they cannot be.
func resolvedPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

// parseFormatTree parses source as format.Source does, for its links and
// <style> elements.
func parseFormatTree(name string, source []byte) (*template.Tree, error) {
	return template.ParseTree(name, name, string(source), nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
		LeftDelim:   "{{",
		RightDelim:  "}}",
	})
}

// writeFileAtomic replaces the file at path with data, keeping its
// permissions: data goes to a temporary file in the same directory, which
// is then renamed over path. A symbolic link is written through: its
// target is replaced and the link kept.
func writeFileAtomic(path string, data []byte) error {
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
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
