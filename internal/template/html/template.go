// Package html provides muxt with an html/template-compatible, context-aware
// escaping template engine built on the position-tracking template parser.
//
// Derived in part from the Go standard library; see NOTICE.txt at the
// repository root.
package html

import (
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"sort"
	"strings"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// FuncMap is the type of the map defining the mapping from names to functions.
type FuncMap = template.FuncMap

// Template is the representation of a parsed template. The *Template
// returned by New, Parse, and related methods is safe for concurrent use
// by multiple goroutines once the template has been fully parsed.
type Template struct {
	// Tree is the parse tree for this template. It is exported so that
	// tools can inspect and modify the AST.
	Tree *template.Tree

	name   string
	common *common
}

type common struct {
	tmpl       map[string]*Template
	funcMap    FuncMap
	missingKey template.MissingKeyMode
	leftDelim  string
	rightDelim string
}

// New allocates a new, undefined template with the given name.
func New(name string) *Template {
	t := &Template{
		name: name,
	}
	t.common = &common{
		tmpl: map[string]*Template{name: t},
	}
	return t
}

// Must is a helper that wraps a call to a function returning (*Template, error)
// and panics if the error is non-nil.
func Must(t *Template, err error) *Template {
	if err != nil {
		panic(err)
	}
	return t
}

// Name returns the name of the template.
func (t *Template) Name() string {
	return t.name
}

// New allocates a new, undefined template associated with the given one
// and with the same delims.
func (t *Template) New(name string) *Template {
	nt := &Template{
		name:   name,
		common: t.common,
	}
	t.common.tmpl[name] = nt
	return nt
}

// Lookup returns the template with the given name that is associated with t.
// It returns nil if there is no such template. As in html/template, a
// template declared with New and never parsed is returned with a nil Tree.
func (t *Template) Lookup(name string) *Template {
	return t.common.tmpl[name]
}

// Templates returns a slice of the templates associated with t, including
// any declared with New and never parsed, as html/template does.
func (t *Template) Templates() []*Template {
	result := make([]*Template, 0, len(t.common.tmpl))
	for _, tmpl := range t.common.tmpl {
		result = append(result, tmpl)
	}
	return result
}

// DefinedTemplates returns a string listing the defined templates,
// prefixed by the string "; defined templates are: ". If there are none,
// it returns the empty string.
func (t *Template) DefinedTemplates() string {
	var names []string
	for _, tmpl := range t.common.tmpl {
		if tmpl.Tree != nil {
			names = append(names, tmpl.name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("; defined templates are: ")
	for i, name := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", name)
	}
	return b.String()
}

// Funcs adds the elements of the argument map to the template's function map.
// It must be called before the template is parsed.
// It panics if a value in the map is not a function with appropriate return
// type. However, it is legal to overwrite elements of the map. The return
// value is the template, so calls can be chained.
func (t *Template) Funcs(funcMap FuncMap) *Template {
	if t.common.funcMap == nil {
		t.common.funcMap = make(FuncMap)
	}
	for k, v := range funcMap {
		t.common.funcMap[k] = v
	}
	return t
}

// Delims sets the action delimiters to the specified strings, to be used in
// subsequent calls to Parse, ParseFiles, or ParseGlob. Nested template
// definitions will inherit the settings. An empty delimiter stands for the
// corresponding default: {{ or }}.
func (t *Template) Delims(left, right string) *Template {
	t.common.leftDelim = left
	t.common.rightDelim = right
	return t
}

// Option sets options for the template. Options are described by
// strings, either a simple string or "key=value". There can be at most one
// equals sign in an option string. If the option string is unrecognized or
// otherwise invalid, Option panics.
//
// Known options:
//
// missingkey: Control the behavior during execution if a map is indexed with
// a key that is not present in the map.
//
//	"missingkey=default" or "missingkey=invalid"
//		The default behavior: Do nothing and continue execution.
//		If printed, the result of the index operation is the string
//		"<no value>".
//	"missingkey=zero"
//		The operation returns the zero value for the map type's element.
//	"missingkey=error"
//		Execution stops immediately with an error.
func (t *Template) Option(opt ...string) *Template {
	for _, o := range opt {
		switch {
		case strings.HasPrefix(o, "missingkey="):
			value := o[len("missingkey="):]
			switch value {
			case "default", "invalid":
				t.common.missingKey = template.MissingKeyInvalid
			case "zero":
				t.common.missingKey = template.MissingKeyZero
			case "error":
				t.common.missingKey = template.MissingKeyError
			default:
				panic(fmt.Sprintf("template: unknown missing key option %q", o))
			}
		default:
			panic(fmt.Sprintf("template: unknown option %q", o))
		}
	}
	return t
}

// Parse parses text as a template body for t. Named template definitions
// ({{define ...}} or {{block ...}} actions) in text define additional
// templates associated with t and are removed from the definition of t itself.
func (t *Template) Parse(text string) (*Template, error) {
	options := t.lexOptions()
	treeSet := make(map[string]*template.Tree)
	tree, err := template.ParseTree(t.name, t.name, text, treeSet, options)
	if err != nil {
		return nil, err
	}
	// html/template records the name as given, with no file behind it;
	// ParseTree takes it for a file path. Blocks registered while parsing
	// copied the root's names, and Extract copies them below.
	tree.ParseName, tree.Filepath = t.name, ""
	for _, tr := range treeSet {
		tr.ParseName, tr.Filepath = t.name, ""
	}
	tree.HTMLMode = true
	template.AnnotateHTMLContexts(tree.Root)
	// Collect templates from both Extract (DefineNodes in HTML mode) and
	// treeSet (block templates registered during parsing).
	for name, tr := range template.Extract(tree) {
		tr.HTMLMode = true
		template.AnnotateHTMLContexts(tr.Root)
		treeSet[name] = tr
	}
	for name, tr := range treeSet {
		if name == t.name {
			continue
		}
		tr.HTMLMode = true
		template.AnnotateHTMLContexts(tr.Root)
		tmpl := t.common.tmpl[name]
		if tmpl == nil {
			tmpl = &Template{
				name:   name,
				common: t.common,
			}
			t.common.tmpl[name] = tmpl
		}
		tmpl.Tree = tr
	}
	t.Tree = tree
	return t, nil
}

// AddParseTree associates the argument parse tree with the template t, giving
// it the specified name. If the template has not been defined, this tree
// becomes its definition. If it has been defined and already has that name,
// the existing definition is replaced; otherwise a new template is created,
// defined, and returned.
func (t *Template) AddParseTree(name string, tree *template.Tree) (*Template, error) {
	tree.HTMLMode = true
	template.AnnotateHTMLContexts(tree.Root)
	tmpl := t.common.tmpl[name]
	if tmpl == nil {
		tmpl = &Template{
			name:   name,
			common: t.common,
		}
		t.common.tmpl[name] = tmpl
	}
	tmpl.Tree = tree
	return tmpl, nil
}

// Clone returns a duplicate of the template, including all associated
// templates. The actual representations are not copied, but the name space
// of associated templates is, so further calls to Parse in the copy will
// add templates to the copy but not to the original.
func (t *Template) Clone() (*Template, error) {
	nt := &Template{
		name: t.name,
	}
	nt.common = &common{
		tmpl:       make(map[string]*Template, len(t.common.tmpl)),
		missingKey: t.common.missingKey,
		leftDelim:  t.common.leftDelim,
		rightDelim: t.common.rightDelim,
	}
	if t.common.funcMap != nil {
		nt.common.funcMap = maps.Clone(t.common.funcMap)
	}
	for k, v := range t.common.tmpl {
		if k == t.name {
			nt.common.tmpl[k] = nt
			nt.Tree = v.Tree
		} else {
			nt.common.tmpl[k] = &Template{
				name:   v.name,
				Tree:   v.Tree,
				common: nt.common,
			}
		}
	}
	return nt, nil
}

// Execute applies a parsed template to the specified data object,
// writing the output to wr.
func (t *Template) Execute(wr io.Writer, data any) error {
	return t.execute(wr, t.name, data)
}

// ExecuteTemplate applies the template associated with t that has the given
// name to the specified data object and writes the output to wr.
func (t *Template) ExecuteTemplate(wr io.Writer, name string, data any) error {
	tmpl := t.Lookup(name)
	if tmpl == nil || tmpl.Tree == nil {
		return fmt.Errorf("html/template: %q is an incomplete or empty template%s", name, t.DefinedTemplates())
	}
	return tmpl.execute(wr, name, data)
}

func (t *Template) execute(wr io.Writer, name string, data any) error {
	tmpl := t.common.tmpl[name]
	if tmpl == nil || tmpl.Tree == nil {
		return fmt.Errorf("html/template: %q is an incomplete or empty template%s", name, t.DefinedTemplates())
	}
	opts := t.execOptions()
	return template.ExecuteTemplate(wr, tmpl.Tree, data, opts...)
}

// ParseFiles parses the named files and associates the resulting templates
// with t. If an error occurs, parsing stops and the returned template is nil;
// otherwise it is t.
func (t *Template) ParseFiles(filenames ...string) (*Template, error) {
	return parseFiles(t, readFileOS, filenames...)
}

// ParseGlob parses the template definitions in the files identified by the
// pattern and associates the resulting templates with t.
func (t *Template) ParseGlob(pattern string) (*Template, error) {
	return parseGlob(t, pattern)
}

// ParseFS is like ParseFiles or ParseGlob but reads from the file system
// fsys instead of the host operating system's file system.
func (t *Template) ParseFS(fsys fs.FS, patterns ...string) (*Template, error) {
	return parseFS(t, fsys, patterns...)
}

// execOptions builds a slice of htt.Option from the template's configuration.
func (t *Template) execOptions() []template.Option {
	opts := []template.Option{template.WithTrust(trustOf)}
	if t.common.funcMap != nil {
		opts = append(opts, template.WithFuncs(t.common.funcMap))
	}
	if t.common.missingKey != template.MissingKeyInvalid {
		opts = append(opts, template.WithMissingKey(t.common.missingKey))
	}
	finder := template.FindTreeFunc(func(name string) (*template.Tree, bool) {
		tmpl := t.common.tmpl[name]
		if tmpl != nil && tmpl.Tree != nil {
			return tmpl.Tree, true
		}
		return nil, false
	})
	opts = append(opts, template.WithFinder(finder))
	return opts
}

// lexOptions builds lex.Options from the template's configuration.
func (t *Template) lexOptions() lex.Options {
	var hasBreak, hasContinue bool
	if t.common.funcMap != nil {
		_, hasBreak = t.common.funcMap["break"]
		_, hasContinue = t.common.funcMap["continue"]
	}
	return lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		LeftDelim:   cmp.Or(t.common.leftDelim, "{{"),
		RightDelim:  cmp.Or(t.common.rightDelim, "}}"),
		BreakOK:     !hasBreak,
		ContinueOK:  !hasContinue,
	}
}
