package template

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/template/lex"
)

type Template[Signature any] struct {
	templates             map[string]*Template[Signature] // shared across associated templates
	name                  string
	tree                  *Tree
	leftDelim, rightDelim string
	functions             map[string]Signature
	missingKey            string // "invalid", "zero", "error", or "" (default)
	text                  bool   // a text/template set: parsed without HTML structure
}

// New returns a template set that parses as html/template does: its
// trees hold HTML structure and are executed in HTML mode.
func New[Signature any](name string) *Template[Signature] {
	t := &Template[Signature]{
		name:      name,
		functions: make(map[string]Signature),
	}
	t.templates = map[string]*Template[Signature]{name: t}
	return t
}

// NewText returns a template set that parses as text/template does: its
// trees are text and actions, with no HTML structure, so text such as
// JSON or prose with a '<' in it parses as it does with text/template.
func NewText[Signature any](name string) *Template[Signature] {
	t := New[Signature](name)
	t.text = true
	return t
}

// IsText reports whether the set parses as text/template does (NewText).
func (t *Template[S]) IsText() bool {
	return t.text
}

// ParseOptions are the lexer options Parse parses text with: the set's
// delimiters, HTML structure unless the set is a text/template set, and
// break and continue as keywords unless the set registers functions of
// those names, as in text/template.
func (t *Template[S]) ParseOptions() lex.Options {
	_, hasBreak := t.functions["break"]
	_, hasContinue := t.functions["continue"]
	return lex.Options{
		EmitComment: true,
		ParseHTML:   !t.text,
		LeftDelim:   cmp.Or(t.leftDelim, "{{"),
		RightDelim:  cmp.Or(t.rightDelim, "}}"),
		BreakOK:     !hasBreak,
		ContinueOK:  !hasContinue,
	}
}

// Names returns the names of every template in the associated set,
// sorted. Tooling (such as LSP completion) uses it to offer the targets
// of {{template}} / {{block}} invocations.
func (t *Template[S]) Names() []string {
	names := make([]string, 0, len(t.templates))
	for name := range t.templates {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (t *Template[S]) Name() string {
	return t.name
}

func (t *Template[S]) Tree() *Tree {
	return t.tree
}

func (t *Template[S]) Functions(functions map[string]S) *Template[S] {
	if t.functions == nil {
		t.functions = make(map[string]S)
	}
	for k, v := range functions {
		t.functions[k] = v
	}
	return t
}

func (t *Template[S]) Option(opt ...string) (*Template[S], error) {
	for _, o := range opt {
		switch {
		case strings.HasPrefix(o, "missingkey="):
			value := o[len("missingkey="):]
			switch value {
			case "invalid", "default":
				// stdlib html/template treats "default" as an alias for
				// "invalid" (both select mapInvalid), so store the canonical form.
				t.missingKey = "invalid"
			case "zero", "error":
				t.missingKey = value
			default:
				return nil, fmt.Errorf("template: unknown missing key option %q", o)
			}
		default:
			return nil, fmt.Errorf("template: unknown option %q", o)
		}
	}
	return t, nil
}

func (t *Template[S]) Delimiters(left, right string) *Template[S] {
	t.leftDelim = left
	t.rightDelim = right
	return t
}

func (t *Template[S]) Lookup(name string) *Template[S] {
	return t.templates[name]
}

func (t *Template[S]) FindTree(name string) (*Tree, bool) {
	if tmpl := t.templates[name]; tmpl != nil {
		return tmpl.tree, true
	}
	return nil, false
}

func (t *Template[S]) Parse(text string) (*Template[S], error) {
	options := t.ParseOptions()
	treeSet := make(map[string]*Tree)
	tree, err := ParseTree(t.name, t.name, text, treeSet, options)
	if err != nil {
		return nil, err
	}
	// text/template records the name as given, with no file behind it;
	// ParseTree takes it for a file path. The trees the parser registered
	// copied the root's names, and Extract copies them from the root.
	tree.ParseName, tree.Filepath = t.name, ""
	for _, tr := range treeSet {
		tr.ParseName, tr.Filepath = t.name, ""
	}
	for name, tr := range DefinedTrees(tree, treeSet) {
		if _, err := t.AddParseTree(name, tr); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (t *Template[S]) AddParseTree(name string, tree *Tree) (*Template[S], error) {
	if !t.text {
		tree.HTMLMode = true
		AnnotateHTMLContexts(tree.Root)
	}
	tmpl := t.templates[name]
	if tmpl == nil {
		tmpl = &Template[S]{
			name:      name,
			templates: t.templates,
			text:      t.text,
		}
		t.templates[name] = tmpl
	}
	// As text/template does, an empty tree does not replace one the
	// template already has.
	if tmpl.tree == nil || !IsEmptyTree(tree.Root) {
		tmpl.tree = tree
	}
	return tmpl, nil
}
