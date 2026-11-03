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
	"regexp"
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
	dir    string // the directory of the package that embeds the file, else the file's own; "" for standard input, which has none
	source []byte
	tree   *template.Tree // nil when the file does not parse
}

// runFormatCommand formats the files config names, or standard input, as
// gofmt does. Problems with a file are reported on stderr, one line each,
// and the other files are still formatted.
func runFormatCommand(cmd *cobra.Command, wd string, config formatConfiguration) error {
	r := formatRun{wd: wd, config: config, stdout: cmd.OutOrStdout(), stderr: cmd.ErrOrStderr(), packages: make(map[string]formatPackage), loads: make(map[string]packageLoad), unread: make(map[string]bool), names: make(map[string]keptNames), firstPass: make(map[*formatFile]formatResult)}
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
		for _, f := range files {
			f.dir = r.embeddingPackageDir(f.path, f.key)
		}
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
	loads          map[string]packageLoad // load.EmbeddedFiles by directory
	unread         map[string]bool        // keys of run files that could not be read, already reported
	names          map[string]keptNames   // by package directory (namesKept)
	firstPass      map[*formatFile]formatResult
	failed         bool
}

// formatResult is what formatSource returned for a file.
type formatResult struct {
	out string
	err error
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
// *.gohtml. A walk leaves out generated files (isGeneratedTemplate); one
// named is formatted.
func (r *formatRun) readFiles(wd string, paths []string) []*formatFile {
	var files []*formatFile
	add := func(name string, walked bool) {
		path := absoluteDir(wd, name)
		source, err := os.ReadFile(path)
		if err != nil {
			r.unread[resolvedPath(path)] = true
			r.report(err.Error())
			return
		}
		if walked && isGeneratedTemplate(source) {
			return
		}
		files = append(files, &formatFile{name: name, path: path, key: resolvedPath(path), source: source})
	}
	for _, name := range paths {
		info, err := os.Stat(absoluteDir(wd, name))
		if err != nil || !info.IsDir() {
			add(name, false)
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
				add(filepath.Join(name, rel), true)
			}
			return nil
		})
		if err != nil {
			r.report(err.Error())
		}
	}
	return files
}

// generatedTemplateLine matches the line that marks a generated
// template: Go's convention (^// Code generated .* DO NOT EDIT\.$) in a
// template comment, trim markers allowed, or in an HTML comment. The
// first group is the template comment's text, the second the HTML
// comment's; isGeneratedTemplate requires the line to be that one
// comment.
var generatedTemplateLine = regexp.MustCompile(`^(?:\{\{-? ?/\* (Code generated .* DO NOT EDIT\.) \*/ ?-?\}\}|<!-- (Code generated .* DO NOT EDIT\.) -->)$`)

// isGeneratedTemplate reports whether the first line of source that is
// not blank is a single comment that marks it generated
// (generatedTemplateLine): the marker never spans two comments.
func isGeneratedTemplate(source []byte) bool {
	for line := range bytes.Lines(source) {
		line = bytes.Trim(line, " \t\r\n")
		if len(line) == 0 {
			continue
		}
		m := generatedTemplateLine.FindSubmatch(line)
		switch {
		case m == nil:
			return false
		case m[1] != nil:
			return !bytes.Contains(m[1], []byte("*/"))
		default:
			return !bytes.Contains(m[2], []byte("-->")) && !bytes.Contains(m[2], []byte("--!>"))
		}
	}
	return false
}

// skipFormatDirectory reports whether a directory walked for templates is
// left out, as the go command leaves it out of ./... patterns.
func skipFormatDirectory(name string) bool {
	return name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// format formats f (formatSource), with the names of its package kept
// as namesKept says, and writes the result where config says.
func (r *formatRun) format(f *formatFile, all []*formatFile) {
	keep := r.namesKept(f.dir, all)
	result, ok := r.firstPass[f]
	if !ok {
		result.out, result.err = r.formatSource(f, all, keep.keep)
	}
	if result.err != nil {
		r.report(result.err.Error())
		return
	}
	r.write(f, []byte(result.out))
}

// formatSource formats f, with the run's other files of f's package and
// the templates that package embeds as context for its author CSS, and
// keeps the template names keep reports (format.KeepNames). A file
// embedded by another package, or by none, is in another package, whose
// CSS does not reach f's. When a template of the package cannot be read
// or parsed, or the package loads with errors, it may link or hold any
// stylesheet, so f is formatted as with a link that does not resolve:
// every element may have unknown CSS. The error is the line to report.
func (r *formatRun) formatSource(f *formatFile, all []*formatFile, keep func(string) bool) (string, error) {
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
		return "", fmt.Errorf("%s: %v", f.name, err)
	}
	styles.MissingSheets = styles.MissingSheets || contextBroken
	out, err := format.Source(f.name, string(f.source), format.Styles(styles), format.KeepNames(keep))
	if err != nil {
		return "", errors.New(formatErrorLine(f.name, err))
	}
	return out, nil
}

// keptNames are the template names of a package whose spellings stay as
// written (format.KeepNames): all of them, or those in kept.
type keptNames struct {
	all  bool
	kept map[string]bool
}

func (k keptNames) keep(canonical string) bool { return k.all || k.kept[canonical] }

// namesKept returns, once per package directory, the template names
// muxt fmt keeps as written so every reference to a template keeps
// naming it: a name a template of the package outside the run spells
// otherwise than canonically, or that two of its define names share
// (format.KeptNames), and a name a file of the run keeps for itself,
// in a muxt:fmt region say, or because it does not format. A file of
// the run with such names is formatted first to find them, and the
// result kept when the names found do not change it. Without a package
// whose templates are all read (standard input, a file no package
// embeds, a package that loads with errors), every name is kept.
func (r *formatRun) namesKept(dir string, all []*formatFile) keptNames {
	if k, ok := r.names[dir]; ok {
		return k
	}
	k := r.findNamesKept(dir, all)
	r.names[dir] = k
	return k
}

func (r *formatRun) findNamesKept(dir string, all []*formatFile) keptNames {
	keepAll := keptNames{all: true}
	if dir == "" {
		return keepAll
	}
	pkg := r.packageIn(dir, all)
	if !pkg.loaded || pkg.broken {
		return keepAll
	}
	var run []*formatFile
	var runTrees []*template.Tree
	inRun := make(map[string]bool)
	for _, f := range all {
		if f.dir != dir {
			continue
		}
		if f.tree == nil {
			return keepAll
		}
		run = append(run, f)
		runTrees = append(runTrees, f.tree)
		inRun[f.key] = true
	}
	var outside []*template.Tree
	for _, e := range pkg.embedded {
		if !inRun[e.key] {
			outside = append(outside, e.tree)
		}
	}
	first := keptNames{kept: format.KeptNames(runTrees, outside)}
	// The names each file would change: those it spells otherwise.
	changes := make(map[*formatFile]map[string]bool)
	var formatted []*template.Tree
	for _, f := range run {
		names := format.KeptNames(nil, []*template.Tree{f.tree})
		maps.DeleteFunc(names, func(name string, _ bool) bool { return first.kept[name] })
		if len(names) == 0 {
			continue
		}
		changes[f] = names
		out, err := r.formatSource(f, all, first.keep)
		r.firstPass[f] = formatResult{out: out, err: err}
		tree := f.tree
		if err == nil {
			if again, err := parseFormatTree(f.name, []byte(out)); err == nil {
				tree = again
			}
		}
		formatted = append(formatted, tree)
	}
	k := keptNames{kept: first.kept}
	for name := range format.KeptNames(nil, formatted) {
		k.kept[name] = true
	}
	for f, names := range changes {
		for name := range names {
			if k.kept[name] {
				delete(r.firstPass, f)
				break
			}
		}
	}
	return k
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
	l := r.load(dir)
	embedded, ok, err := l.files, l.ok, l.err
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

// packageLoad is what load.EmbeddedFiles returned for a directory.
type packageLoad struct {
	files []string
	ok    bool
	err   error
}

// load loads the package in dir once per run, for embeddingPackageDir to
// probe and packageIn to use. It reports nothing: only the errors of the
// package a file belongs to are reported, by packageIn.
func (r *formatRun) load(dir string) packageLoad {
	if l, ok := r.loads[dir]; ok {
		return l
	}
	var l packageLoad
	l.files, l.ok, l.err = load.EmbeddedFiles(dir)
	r.loads[dir] = l
	return l
}

// embeddingPackageDir is the directory of the package that embeds the file
// at path, whose key is its path with symbolic links resolved: the nearest
// directory, from the file's own up to the one holding go.mod, whose
// package embeds key. A //go:embed pattern reaches only its package's
// directory and those below it, in the same module, so no other directory
// can. A file no package embeds, or one outside any module, belongs to its
// own directory.
func (r *formatRun) embeddingPackageDir(path, key string) string {
	own := filepath.Dir(path)
	root, ok := moduleRoot(own)
	if !ok {
		return own
	}
	for dir := own; ; dir = filepath.Dir(dir) {
		l := r.load(dir)
		for _, file := range l.files {
			if resolvedPath(file) == key {
				return dir
			}
		}
		if dir == root || filepath.Dir(dir) == dir {
			return own
		}
	}
}

// moduleRoot is the nearest directory, from dir up, that holds a go.mod
// file.
func moduleRoot(dir string) (string, bool) {
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
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
